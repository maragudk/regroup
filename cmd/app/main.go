package main

import (
	"cmp"
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"net"
	"os"
	"time"
	_ "time/tzdata"

	"maragu.dev/env"
	"maragu.dev/errors"
	"maragu.dev/glue/app"
	"maragu.dev/glue/aws"
	"maragu.dev/glue/email"
	"maragu.dev/glue/email/postmark"
	gluehttp "maragu.dev/glue/http"
	gluejobs "maragu.dev/glue/jobs"
	"maragu.dev/glue/s3"
	"maragu.dev/glue/sql"
	"maragu.dev/glue/sqlitestore"
	"maragu.dev/gomponents"

	"app/html"
	"app/http"
	"app/jobs"
	"app/model"
	"app/service"
	"app/sqlite"
)

func main() {
	app.Start(start)
}

// defaultGroupName when the configuration names none. The preview build names its made-up group.
var defaultGroupName = "Our group"

// setupPreview is set only in the preview build, by the file behind the preview build tag. It seeds an
// empty database, and returns what adds the preview's routes and what keeps the emails the app would
// have sent, which the preview sends instead of sending them.
var setupPreview func(ctx context.Context, log *slog.Logger, db *sqlite.Database, now func() time.Time, group model.Group) (func(*gluehttp.Router), email.Sender, error)

// previewLink is set only in the preview build, by the file behind the preview build tag. It returns the
// link to the preview's list of people that pages show.
var previewLink func(signedIn bool) gomponents.Node

func start(ctx context.Context, log *slog.Logger, eg app.Goer) error {
	databaseLog := log.With("component", "sql.Database")

	jobTimeout := env.GetDurationOrDefault("JOB_QUEUE_TIMEOUT", 10*time.Second)

	db := sqlite.NewDatabase(sqlite.NewDatabaseOptions{
		H: sql.NewHelper(sql.NewHelperOptions{
			JobQueue: sql.JobQueueOptions{
				Timeout: jobTimeout,
			},
			Log: databaseLog,
			SQLite: sql.SQLiteOptions{
				Path: env.GetStringOrDefault("DATABASE_PATH", "app.db"),
			},
		}),
		Log: databaseLog,
	})
	if err := db.H.Connect(ctx); err != nil {
		return errors.Wrap(err, "error connecting to database")
	}

	if err := db.H.MigrateUp(ctx); err != nil {
		return errors.Wrap(err, "error migrating database")
	}

	if env.GetBoolOrDefault("AWS_ENDPOINT_URL_LOCAL", false) {
		s3Port, err := port("versitygw")
		if err != nil {
			return err
		}
		if err := os.Setenv("AWS_ENDPOINT_URL", fmt.Sprint("http://localhost:", s3Port)); err != nil {
			return errors.Wrap(err, "error setting AWS endpoint URL")
		}
	}

	awsConfig, err := aws.LoadDefaultConfig(ctx)
	if err != nil {
		return err
	}

	bucket := s3.NewBucket(s3.NewBucketOptions{
		Config:    awsConfig,
		Name:      env.GetStringOrDefault("S3_BUCKET_NAME", "bucket"),
		PathStyle: env.GetBoolOrDefault("S3_PATH_STYLE", false),
	})

	runner := gluejobs.NewRunner(gluejobs.NewRunnerOpts{
		Log:   log.With("component", "jobs.Runner"),
		Queue: db.H.JobsQ,
	})

	defaultAddress := ":8080"
	if env.GetBoolOrDefault("SERVER_ADDRESS_LOCAL", false) {
		appPort, err := port("app")
		if err != nil {
			return err
		}
		defaultAddress = fmt.Sprint(":", appPort)
	}
	address := env.GetStringOrDefault("SERVER_ADDRESS", defaultAddress)
	_, serverPort, _ := net.SplitHostPort(address)
	baseURL := env.GetStringOrDefault("BASE_URL", "http://localhost:"+serverPort)

	var sender email.Sender = postmark.NewSender(postmark.NewSenderOptions{
		AppName:                   env.GetStringOrDefault("APP_NAME", "App"),
		BaseURL:                   baseURL,
		Emails:                    html.EmailTemplates(),
		Key:                       env.GetStringOrDefault("POSTMARK_KEY", ""),
		Log:                       slog.New(withoutRecipients(log.Handler())).With("component", "email.Sender"),
		MarketingEmailAddress:     model.EmailAddress(env.GetStringOrDefault("MARKETING_EMAIL_ADDRESS", "marketing@example.com")),
		MarketingEmailName:        env.GetStringOrDefault("MARKETING_EMAIL_NAME", "Marketing"),
		ReplyToEmailAddress:       model.EmailAddress(env.GetStringOrDefault("REPLY_TO_EMAIL_ADDRESS", "support@example.com")),
		ReplyToEmailName:          env.GetStringOrDefault("REPLY_TO_EMAIL_NAME", "Support"),
		TransactionalEmailAddress: model.EmailAddress(env.GetStringOrDefault("TRANSACTIONAL_EMAIL_ADDRESS", "transactional@example.com")),
		TransactionalEmailName:    env.GetStringOrDefault("TRANSACTIONAL_EMAIL_NAME", "Transactional"),
	})

	group, err := model.NewGroup(cmp.Or(env.GetStringOrDefault("GROUP_NAME", ""), defaultGroupName), env.GetStringOrDefault("GROUP_TIME_ZONE", ""))
	if err != nil {
		return errors.Wrap(err, "error configuring group")
	}

	firstAdmin, err := parseFirstAdmin(env.GetStringOrDefault("FIRST_ADMIN_EMAIL", ""))
	if err != nil {
		return err
	}

	var previewInjector func(*gluehttp.Router)
	if setupPreview != nil {
		previewInjector, sender, err = setupPreview(ctx, log, db, time.Now, group)
		if err != nil {
			return errors.Wrap(err, "error setting up preview")
		}
	}

	svc := service.NewFat(service.NewFatOptions{
		Log: log.With("component", "service.Fat"),
	})
	service.Setup(svc, bucket, db, sender, time.Now, rand.Reader, group, firstAdmin)

	jobs.Register(runner, jobs.RegisterOpts{
		Emails: svc,
	})

	routerInjector := http.InjectHTTPRouter(log, svc, http.InjectHTTPRouterOptions{
		Group:  group,
		Now:    time.Now,
		Random: rand.Reader,
		Extra:  previewLink,
	})
	if previewInjector != nil {
		appInjector := routerInjector
		routerInjector = func(r *gluehttp.Router) {
			appInjector(r)
			previewInjector(r)
		}
	}

	store, err := sqlitestore.New(ctx, db.H.DB.DB)
	if err != nil {
		return errors.Wrap(err, "error creating sqlite session store")
	}

	server := gluehttp.NewServer(gluehttp.NewServerOptions{
		Address:            address,
		BaseURL:            baseURL,
		CSP:                http.CSP(env.GetBoolOrDefault("CSP_ALLOW_UNSAFE_INLINE", false)),
		HTMLPage:           html.Page,
		HTTPRouterInjector: routerInjector,
		Log:                log.With("component", "http.Server"),
		SecureCookie:       env.GetBoolOrDefault("SECURE_COOKIE", true),
		SessionStore:       store,
		UserActiveChecker:  db,
	})

	eg.Go(func() error {
		return server.Start(ctx)
	})

	eg.Go(func() error {
		runner.Start(ctx)
		return nil
	})

	return nil
}

// parseFirstAdmin from the configuration: the email address of the person who can sign in and join as
// the first admin while the group has nobody in it. Empty means nobody can.
func parseFirstAdmin(s string) (model.EmailAddress, error) {
	if s == "" {
		return "", nil
	}
	e, err := model.ParseEmailAddress(s)
	if err != nil {
		return "", errors.New("FIRST_ADMIN_EMAIL is not an email address")
	}
	return e, nil
}
