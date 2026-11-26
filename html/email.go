package html

import (
	"embed"
	"html/template"
	"io/fs"
	"strings"
)

//go:embed emails
var emails embed.FS

// EmailTemplates the app sends: the layout, layout.html, with the placeholder {{body}} for the body, and
// the body of each kind of email, such as sign-in.html. Placeholders are words in double curly braces,
// such as {{token}} for the token in a link.
func EmailTemplates() fs.FS {
	templates, err := fs.Sub(emails, "emails")
	if err != nil {
		panic(err)
	}
	return templates
}

// EmailBody of the email template with the given name, without the layout, with its placeholders replaced
// by the given keywords, each escaped as HTML, in one pass, so that a keyword's value is never read as a
// placeholder. Placeholders without a keyword are left as they are. Returns an error if there is no such
// template.
func EmailBody(name string, keywords map[string]string) (string, error) {
	body, err := fs.ReadFile(EmailTemplates(), name+".html")
	if err != nil {
		return "", err
	}

	var oldnew []string
	for k, v := range keywords {
		oldnew = append(oldnew, "{{"+k+"}}", template.HTMLEscapeString(v))
	}
	return strings.NewReplacer(oldnew...).Replace(string(body)), nil
}
