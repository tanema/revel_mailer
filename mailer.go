package revel_mailer

import (
  "github.com/robfig/revel"
  "net/smtp"
  "bytes"
  "io/ioutil"
  "io"
  "mime/multipart"
  "fmt"
  "path"
  "runtime"
  "reflect"
  "strings"
  "net"
  "crypto/tls"
)

const CRLF = "\r\n"

type Mailer struct {
  to, cc, bcc []string
  template string
  renderargs map[string]interface{}
  host, from, username string
  port int
  tls bool
}

type H map[string]interface{}

func (m *Mailer) do_config(){
  ok := true
  m.host, ok = revel.Config.String("mail.host")
  if !ok {
    revel.ERROR.Println("mail host not set")
  }
  m.port, ok = revel.Config.Int("mail.port")
  if !ok {
    revel.ERROR.Println("mail port not set")
  }
  m.from, ok = revel.Config.String("mail.from") 
  if !ok {
    revel.ERROR.Println("mail.from not set")
  }
  m.username, ok = revel.Config.String("mail.username") 
  if !ok {
    revel.ERROR.Println("mail.username not set")
  }
  m.tls = revel.Config.BoolDefault("mail.tls", false) 
}

func (m *Mailer) getClient() (*smtp.Client, error) {
  var c *smtp.Client
  if m.tls == true {
    conn, err := tls.Dial("tcp", fmt.Sprintf("%s:%d", m.host, m.port), nil)
    if err != nil {
      return nil, err
    }
    c, err = smtp.NewClient(conn, m.host)
    if err != nil {
      return nil, err
    }
  } else {
    conn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", m.host, m.port))
    if err != nil {
      return nil, err
    }
    c, err = smtp.NewClient(conn, m.host)
    if err != nil {
      return nil, err
    }
  }
  return c, nil
}

func (m *Mailer) Send(mail_args map[string]interface{}) error {
  m.renderargs = mail_args
  pc, _, _, _ := runtime.Caller(1)
  names := strings.Split(runtime.FuncForPC(pc).Name(), ".")
  m.template =  names[len(names)-2] + "/" + names[len(names)-1]
  m.do_config()

  c, err := m.getClient()
  if err != nil {
    return err
  }

  if ok, _ := c.Extension("STARTTLS"); ok {
    if err = c.StartTLS(nil); err != nil {
      return err
    }
  }

  if err = c.Auth(smtp.PlainAuth(m.from, m.username, getPassword(), m.host)); err != nil {
    return err
  }

  if err = c.Mail(m.username); err != nil {
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
  email_pwd_path := path.Clean(path.Join(revel.BasePath, "email.pwd"))

  if revel.RunMode == "dev" {
    revel.INFO.Println(email_pwd_path)
  }

  password_byte, err := ioutil.ReadFile(email_pwd_path)
  if err != nil {
      password = revel.Config.StringDefault("mail.password", "")
  }else{
    password = string(password_byte)
  }
  if password == "" {
    revel.ERROR.Println("mail password not set")
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

