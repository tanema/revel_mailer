package revel_mailer

import (
  "github.com/robfig/revel"
  "net/smtp"
  "bytes"
  "io/ioutil"
  "io"
  "os"
  "mime/multipart"
  "fmt"
  "path"
  "runtime"
  "reflect"
  "strings"
)

const CRLF = "\r\n"

type Mailer struct {
  to, cc, bcc []string
  template string
  renderargs map[string]interface{}
}

type H map[string]interface{}

func (m *Mailer) Send(mail_args map[string]interface{}) error {
  m.renderargs = mail_args
  pc, _, _, _ := runtime.Caller(1)
  names := strings.Split(runtime.FuncForPC(pc).Name(), ".")
  m.template =  names[len(names)-2] + "/" + names[len(names)-1]

  host := revel.Config.StringDefault("mail.host", "")
  full_url := fmt.Sprintf("%s:%d", host, revel.Config.IntDefault("mail.port", 25))
  c, err := smtp.Dial(full_url)
  if err != nil {
    return err
  }
  if ok, _ := c.Extension("STARTTLS"); ok {
    if err = c.StartTLS(nil); err != nil {
      return err
    }
  }
  if err = c.Auth(smtp.PlainAuth(
      revel.Config.StringDefault("mail.from", ""),
      revel.Config.StringDefault("mail.username", ""),
      getPassword(),
      host,
    )); err != nil {
       return err
  }
  if err = c.Mail(revel.Config.StringDefault("mail.username", "")); err != nil {
    return err
  }

  if mail_args["to"] != nil {
    m.to = makeSAFI(mail_args["to"])
  }
  if mail_args["cc"] != nil {
    m.cc = makeSAFI(mail_args["cc"])
  }
  if mail_args["bcc"] != nil {
    m.bcc = makeSAFI(mail_args["bcc"])
  }

  if len(m.to) + len(m.cc) + len(m.bcc) == 0 {
    return fmt.Errorf("Cannot send email without recipients")
  }

  recipients := append(m.to, append(m.cc, m.bcc...)...)
  for _, addr := range recipients {
    if err = c.Rcpt(addr); err != nil {
      return err
    }
  }
  w, err := c.Data()
  if err != nil {
    return err
  }

  mail, err := m.renderMail(w)
  if err != nil {
    return err
  }

  if revel.RunMode == "dev" {
    fmt.Println(string(mail))
  }

  _, err = w.Write(mail)
  if err != nil {
    return err
  }
  err = w.Close()
  if err != nil {
    return err
  }
  return c.Quit()
}

func (m *Mailer) renderMail(w io.WriteCloser) ([]byte, error) {
  multi := multipart.NewWriter(w)

  body, err := m.renderBody(multi)
  if err != nil {
    return nil, err
  }

  mail := []string{
    "Subject: " + reflect.ValueOf(m.renderargs["subject"]).String(),
    "From: " + revel.Config.StringDefault("mail.from", revel.Config.StringDefault("mail.username", "")),
    "To: " + strings.Join(m.to, ","),
    "Bcc: " + strings.Join(m.bcc, ","),
    "Cc: " + strings.Join(m.cc, ","),
    "Content-Type: multipart/alternative; boundary=" + multi.Boundary(),
    CRLF,
    body,
    "--" + multi.Boundary() + "--",
    CRLF,
  }

  return []byte(strings.Join(mail, CRLF)), nil
}

func (m *Mailer) renderBody(multi *multipart.Writer) (string, error) {
  body := ""
  contents := map[string]string{"plain": m.renderTemplate("txt"), "html": m.renderTemplate("html")}
  for k, v := range contents {
    if v != "" {
      body += "--" + multi.Boundary() + CRLF + "Content-Type: text/" + k + "; charset=UTF-8" + CRLF + CRLF + v + CRLF + CRLF
    }
  }

  if body == "" {
    return body, fmt.Errorf("No valid mail templates were found with the names %s.[html|txt]", m.template)
  }

  return body, nil
}

func (m *Mailer) renderTemplate(mime string) string {
  var body bytes.Buffer
  template, err := revel.MainTemplateLoader.Template(m.template + "." + mime)
  if template == nil || err != nil {
    if revel.RunMode == "dev" {
      revel.ERROR.Println(err)
    }
    return ""
  } else {
    template.Render(&body, m.renderargs)
  }
  return body.String()
}

func getPassword() string {
  password := ""
  wd, _ := os.Getwd()
  email_pwd_path := path.Clean(path.Join(wd, "./email.pwd"))
  password_byte, err := ioutil.ReadFile(email_pwd_path)
  if err != nil {
      password = revel.Config.StringDefault("mail.password", "")
  }else{
    password = string(password_byte)
  }
  return password
}

func makeSAFI(intfc interface{}) []string{
  result := []string{}
  slicev := reflect.ValueOf(intfc)
  for i := 0; i < slicev.Len(); i++ {
    result = append(result, slicev.Index(i).String())
  }
  return result
}

