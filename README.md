Revel Mailer
============

A (as of yet) simple mailer for revel to use revel's config and template engine

Usage
-----
- first add in the setting to conf/app.conf

```

mail.host = smtp.example.com
mail.port = 25 (defaults to 25)
mail.from = Your sender name
mail.username = yourmail@mail.com

```
- create a mailers folder under app/
- create a mailer, for example `user_mailer.go` would look like this

```go

package mailers

import "github.com/tanema/revel_mailer"

type UserMailer struct {
  revel_mailer.Mailer
}
//revel_mailer.H is just a helper for map[string]interface{}
func (u UserMailer) SendReport(reported_id string) error {
  u.Attach("/path/to/file")

  return u.Send(revel_mailer.H{
            "subject": "a signature has been reported",
            "to": []string{"email@email.com"},
            "reported_id": reported_id,
          })
}

```

- create the views for this action in app/views/UserMailer/
- You can create both SendReport.html and SendReport.txt
- All of the arguments you pass into Send are pssed to those templates for example this is `SendReport.html`

```html

  <h1>{{.subject}}</h1>
  this user with the id: <b>{{.reported_id}}</b> has been reported

```

- then in your controller (or wherever) call it like this

```go

err := mailers.UserMailer{}.SendReport(id)

```
