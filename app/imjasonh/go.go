package imjasonh

import (
	"appengine"
	"appengine/datastore"
	"appengine/user"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"time"
)

type Shortcut struct {
	URL     string
	User    string
	Created time.Time
}

const (
	form = `<html><body>
<form action="/go" method="POST">
  <label for="key">Key</label>
  <input type="text" name="key" id="key"></input><br />

  <label for="url">URL</label>
  <input type="text" name="url" id="url"></input><br />

  <input type="submit" value="Submit"></input> 
</form>
<ul>
{{range .Shortcuts}}
  <li><a href="/go/{{.Key}}">/go/{{.Key}}</a> -> {{.URL}} (created {{.Created}}</li>
{{else}}
  <li>You have not created any shortcuts yet.</li>
{{end}}
</ul>
<a href="{{.LogoutURL}}">Log out</a>
</body></html>
`

	login = `<html><body>
  <a href="{{.LoginURL}}">Log in to create shortcuts</a>
</body></html>`

	path = "/go"
)

func init() {
	http.HandleFunc("/go", newGo)
	http.HandleFunc("/go/", go_)
}

func newGo(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)

	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	usr := user.Current(c)
	if usr == nil {
		loginURL, err := user.LoginURL(c, path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		t := template.Must(template.New("login").Parse(login))
		t.Execute(w, map[string]string{
			"LoginURL": loginURL,
		})
		return
	}

	if r.Method == "GET" {
		// Display new shortcut form
		logout, err := user.LogoutURL(c, r.URL.String())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		q := datastore.NewQuery("Shortcut").
			Filter("User =", usr.Email).
			Order("-Created").
			Limit(100)
		cnt, err := q.Count(c)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		scuts := make([]map[string]interface{}, cnt)
		i := 0
		for t := q.Run(c); ; {
			var s Shortcut
			key, err := t.Next(&s)
			if err != nil {
				if err == datastore.Done {
					break
				}
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			scuts[i] = map[string]interface{}{
				"Key":     key.StringID(),
				"URL":     s.URL,
				"Created": s.Created.String(),
			}
			i++
		}

		t := template.Must(template.New("form").Parse(form))
		t.Execute(w, map[string]interface{}{
			"LogoutURL": logout,
			"Shortcuts": scuts,
		})
	} else if r.Method == "POST" {
		// Save shortcut
		k := r.FormValue("key")
		u := r.FormValue("url")

		if k == "" || u == "" {
			http.Error(w, "Must provide key and URL", http.StatusBadRequest)
			return
		}
		parsed, err := url.Parse(u)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if !parsed.IsAbs() {
			http.Error(w, "URL must be absolute", http.StatusBadRequest)
			return
		}
		if r.URL.IsAbs() && parsed.Host == r.URL.Host {
			http.Error(w, "URL cannot link to this app", http.StatusBadRequest)
			return
		}

		dsKey := datastore.NewKey(c, "Shortcut", k, 0, nil)

		// Prevent overwriting existing shortcuts owned by other users
		var s Shortcut
		err = datastore.Get(c, dsKey, &s)

		if err == nil && s.User != usr.Email {
			http.Error(w, "Shortcut is owned by another user", http.StatusBadRequest)
			return
		}
		if err != nil && err != datastore.ErrNoSuchEntity {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		s = Shortcut{
			URL:     u,
			User:    usr.Email,
			Created: time.Now(),
		}
		if _, err := datastore.Put(c, dsKey, &s); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(w, "Success!")
	} else {
		// TODO: Support DELETE?
		http.Error(w, "Unsupported method", http.StatusMethodNotAllowed)
	}
}

// go_ redirects to a previsouly-defined URL by going to /go/<key>
func go_(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)

	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	key := r.URL.Path[len(path)+1:]

	var s Shortcut
	dsKey := datastore.NewKey(c, "Shortcut", key, 0, nil)
	if err := datastore.Get(c, dsKey, &s); err != nil {
		if err == datastore.ErrNoSuchEntity {
			http.Error(w, "No such shortcut exists", http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	http.Redirect(w, r, s.URL, http.StatusMovedPermanently)
}
