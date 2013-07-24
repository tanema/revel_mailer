package revel_mailer

import (
  "github.com/robfig/revel"
  "encoding/base64"
  "mime/multipart"
  "path/filepath"
  "crypto/tls"
  "io/ioutil"
  "net/smtp"
  "runtime"
  "strings"
  "reflect"
  "bytes"
  "path"
  "net"
  "fmt"
  "io"
  "os"
)

const CRLF = "\r\n"

type Mailer struct {
  to, cc, bcc []string
  template string
  renderargs map[string]interface{}
  address, from, username string
  port int
  tls, debug, concurrent bool
  attachments map[string][]byte
}

type H map[string]interface{}

func (m *Mailer) do_config(){
  ok := true
  m.address, ok = revel.Config.String("mail.address")
  if !ok {
    revel.ERROR.Println("mail address not set")
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
  m.debug = revel.Config.BoolDefault("mail.debug", false) 
  m.concurrent = revel.Config.BoolDefault("mail.concurrent", true) 
}

func (m *Mailer) getClient() (*smtp.Client, error) {
  var c *smtp.Client
  if m.tls == true {
    conn, err := tls.Dial("tcp", fmt.Sprintf("%s:%d", m.address, m.port), nil)
    if err != nil {
      return nil, err
    }
    c, err = smtp.NewClient(conn, m.address)
    if err != nil {
      return nil, err
    }
  } else {
    conn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", m.address, m.port))
    if err != nil {
      return nil, err
    }
    c, err = smtp.NewClient(conn, m.address)
    if err != nil {
      return nil, err
    }
  }
  return c, nil
}

func (m *Mailer) Attach(file string) error {
  if m.attachments == nil {
    m.attachments = make(map[string][]byte)
  }

  b, err := ioutil.ReadFile(file)
  if err != nil {
    return err
  }

  _, fileName := filepath.Split(file)
  m.attachments[fileName] = b
  return nil
}

func (m *Mailer) Send(mail_args map[string]interface{}) error {
  m.renderargs = mail_args
  pc, _, _, _ := runtime.Caller(1)
  names := strings.Split(runtime.FuncForPC(pc).Name(), ".")
  m.template =  names[len(names)-2] + "/" + names[len(names)-1]
  m.do_config()

  if mail_args["to"] != nil {
    m.to = makeSAFI(mail_args["to"])
  }
  if mail_args["cc"] != nil {
    m.cc = makeSAFI(mail_args["cc"])
  }
  if mail_args["bcc"] != nil {
    m.bcc = makeSAFI(mail_args["bcc"])
  }

  if m.debug {
    return m.sendDebug()
  }else{
    if m.concurrent {
      go m.send()
      return nil
    }else{
      return m.send()
    }
  }
}

func (m *Mailer) sendDebug() error {
  mail, err := m.renderMail(nil)
  if err != nil {
    return err
  }
  fmt.Println(string(mail))
  return nil
}

func (m *Mailer) send() error {
  c, err := m.getClient()
  if err != nil {
    return err
  }

  if ok, _ := c.Extension("STARTTLS"); ok {
    if err = c.StartTLS(nil); err != nil {
      return err
    }
  }

  if err = c.Auth(smtp.PlainAuth(m.from, m.username, m.getPassword(), m.address)); err != nil {
    return err
  }

  if err = c.Mail(m.username); err != nil {
    return err
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
  multi := &multipart.Writer{}
  if w != nil {
    multi = multipart.NewWriter(w)
  }else{
    multi = multipart.NewWriter(bytes.NewBufferString(""))
  }

  body, err := m.renderBody(w)
  if err != nil {
    return nil, err
  }

  from := ""
  if m.renderargs["from"] == nil {
    from = revel.Config.StringDefault("mail.from", revel.Config.StringDefault("mail.username", ""))
  }else{
    from = reflect.ValueOf(m.renderargs["from"]).String()
  }

  mail := []string{
    "Subject: " + reflect.ValueOf(m.renderargs["subject"]).String(),
    "From: " + from,
    "To: " + strings.Join(m.to, ","),
    "Bcc: " + strings.Join(m.bcc, ","),
    "Cc: " + strings.Join(m.cc, ","),
    "MIME-Version: 1.0",
    "Content-Type: multipart/mixed; boundary=" + multi.Boundary(),
    "Content-Transfer-Encoding: 7bit",
    CRLF, CRLF, CRLF,
    m.renderAttachments(multi.Boundary()),
    CRLF,
    "--" + multi.Boundary(),
    body,
    CRLF,
    "--" + multi.Boundary() + "--",
    CRLF,
  }

  return []byte(strings.Join(mail, CRLF)), nil
}

func (m *Mailer) renderBody(w io.WriteCloser) (string, error) {
  multi := &multipart.Writer{}
  if w != nil {
    multi = multipart.NewWriter(w)
  }else{
    multi = multipart.NewWriter(bytes.NewBufferString(""))
  }

  body := bytes.NewBuffer(nil)

  body.WriteString("Mime-Version: 1.0" + CRLF)
  body.WriteString("Content-Type: multipart/alternative; boundary=" + multi.Boundary() + "; charset=UTF-8" + CRLF)
  body.WriteString("Content-Transfer-Encoding: 7bit" + CRLF)

  template_count := 0
  contents := map[string]string{"plain": m.renderTemplate("txt"), "html": m.renderTemplate("html")}
  for k, v := range contents {
    if v != "" {
      body.WriteString("--" + multi.Boundary() + CRLF + "Content-Type: text/" + k + "; charset=UTF-8" + CRLF + "Content-Transfer-Encoding: quoted-printable" + CRLF + CRLF + v + CRLF + CRLF)
      template_count++
    }
  }

  body.WriteString(m.renderAttachments(multi.Boundary()))

  body.WriteString("--" + multi.Boundary() + "--")

  if template_count == 0 {
    return "", fmt.Errorf("No valid mail templates were found with the names %s.[html|txt]", m.template)
  }

  return body.String(), nil
}

func (m *Mailer) renderAttachments(boundary string) string {
  body := bytes.NewBuffer(nil)

  if len(m.attachments) > 0 {
    for k, v := range m.attachments {
      body.WriteString("--" + boundary + CRLF)
      body.WriteString("Content-Type: application/octet-stream"+CRLF)
      body.WriteString("Content-Transfer-Encoding: base64"+CRLF)
      body.WriteString("Content-Disposition: attachment; filename=\"" + k + "\"" + CRLF + CRLF)

      b := make([]byte, base64.StdEncoding.EncodedLen(len(v)))
      base64.StdEncoding.Encode(b, v)
      body.Write(b)
      body.WriteString(CRLF + CRLF)
    }
  }

  return body.String()
}

func (m *Mailer) renderTemplate(mime string) string {
  var body bytes.Buffer
  template, err := revel.MainTemplateLoader.Template(m.template + "." + mime)
  if template == nil || err != nil {
    if m.debug {
      revel.ERROR.Println(err)
    }
    return ""
  } else {
    template.Render(&body, m.renderargs)
  }
  return body.String()
}

func (m *Mailer) getPassword() string {
  password := ""
  email_pwd_path := path.Clean(path.Join(revel.BasePath, "email.pwd"))

  if m.debug {
    revel.INFO.Println(email_pwd_path)
  }

  password_byte, err := ioutil.ReadFile(email_pwd_path)
  if err != nil {
      if os.Getenv("REVEL_EMAIL_PW") == "" {
        password = revel.Config.StringDefault("mail.password", "")
      }else{
        password = os.Getenv("REVEL_EMAIL_PW")
      }
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

