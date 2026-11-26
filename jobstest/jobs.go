// Package jobstest provides testing helpers for jobs on the queue.
package jobstest

import (
	"bytes"
	"encoding/gob"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"testing"

	"maragu.dev/glue/jobs"

	"app/model"
	"app/sqlite"
)

// Job taken off the queue: its name and its data, as the code that created it gave them.
type Job struct {
	Name model.JobName
	Data []byte
}

// message is how the queue's job runner encodes a job, by name and message, with gob.
type message struct {
	Name    string
	Message []byte
}

// envelope is how glue wraps a job's data with its trace context.
type envelope struct {
	Body         jsontext.Value
	TraceContext map[string]string
}

// Take every job on the queue of regular jobs off it, in the order they were created, without running
// them. The data of each is what the code that created it gave, without the trace context glue adds.
func Take(t testing.TB, db *sqlite.Database) []Job {
	t.Helper()

	var taken []Job
	for {
		m, err := db.H.JobsQ.Receive(t.Context())
		if err != nil {
			t.Fatal("error receiving job:", err)
		}
		if m == nil {
			return taken
		}
		if err := db.H.JobsQ.Delete(t.Context(), m.ID); err != nil {
			t.Fatal("error deleting job:", err)
		}

		var jm message
		if err := gob.NewDecoder(bytes.NewReader(m.Body)).Decode(&jm); err != nil {
			t.Fatal("error decoding job:", err)
		}
		data := jm.Message
		var e envelope
		if err := json.Unmarshal(jm.Message, &e); err == nil && len(e.Body) > 0 && e.TraceContext != nil {
			data = e.Body
		}
		taken = append(taken, Job{Name: model.JobName(jm.Name), Data: data})
	}
}

// Run every job on the queue of regular jobs with the given functions, in the order they were created,
// until there are none, including jobs that jobs create, and return what ran. A job without a function,
// or one that fails, fails the test.
func Run(t testing.TB, db *sqlite.Database, funcs map[model.JobName]jobs.Func) []Job {
	t.Helper()

	var ran []Job
	for {
		taken := Take(t, db)
		if len(taken) == 0 {
			return ran
		}
		for _, j := range taken {
			f, ok := funcs[j.Name]
			if !ok {
				t.Fatalf("no function for job %q", j.Name)
			}
			if err := f(t.Context(), j.Data); err != nil {
				t.Fatalf("error running job %q: %v", j.Name, err)
			}
			ran = append(ran, j)
		}
	}
}
