package web

import (
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	e "iele/internal/err"
)

const MaxBody = 4 * 1024 * 1024

type Req struct {
	URL     string
	Method  string
	Headers map[string]string
	Body    io.Reader
	Timeout time.Duration
	UA      string
	Proxy   string
	Client  *http.Client
}

type Res struct {
	Code   int
	Header http.Header
	Body   io.ReadCloser
}

type Data struct {
	Code   int
	Header http.Header
	Body   []byte
}

type Field struct {
	Name string
	Val  string
}

type File struct {
	Field string
	Name  string
	MIME  string
	Path  string
}

// Timeout defaults to 10s if not specified.
func Do(r Req) (Res, error) {
	if r.URL == "" {
		return Res{}, e.New("", e.Call, "web:req", "url_missing")
	}
	if r.Method == "" {
		r.Method = http.MethodGet
	}
	if r.Timeout == 0 {
		r.Timeout = 10 * time.Second
	}

	client := r.Client
	if client == nil {
		client = &http.Client{Timeout: r.Timeout}
	}
	if r.Proxy != "" {
		pURL, err := url.Parse(r.Proxy)
		if err != nil {
			return Res{}, e.Wrap("", e.Prov, "web:proxy", err)
		}
		if r.Client != nil {
			return Res{}, e.New("", e.Call, "web:proxy", "client_proxy")
		}
		tx := http.DefaultTransport.(*http.Transport).Clone()
		tx.Proxy = http.ProxyURL(pURL)
		client.Transport = tx
	}

	req, err := http.NewRequest(r.Method, r.URL, r.Body)
	if err != nil {
		return Res{}, e.Wrap("", e.Prov, "web:req", err)
	}

	for k, v := range r.Headers {
		req.Header.Set(k, v)
	}
	if r.UA != "" {
		req.Header.Set("User-Agent", r.UA)
	}

	resp, err := client.Do(req)
	if err != nil {
		return Res{}, e.Wrap("", e.Trans, "web:do", err)
	}

	return Res{
		Code:   resp.StatusCode,
		Header: resp.Header.Clone(),
		Body:   resp.Body,
	}, nil
}

// Read closes res.Body after reading it. max defaults to MaxBody when zero.
func Read(res Res, max int64) (Data, error) {
	if res.Body == nil {
		return Data{}, e.New("", e.Call, "web:read", "body_missing")
	}
	if max < 0 {
		return Data{}, e.New("", e.Call, "web:read", "bad_cap")
	}
	if max == 0 {
		max = MaxBody
	}
	defer res.Body.Close()

	body, err := io.ReadAll(io.LimitReader(res.Body, max+1))
	if err != nil {
		return Data{}, e.Wrap("", e.Trans, "web:read", err)
	}
	if int64(len(body)) > max {
		return Data{}, e.New("", e.Cap, "web:read", "body_cap")
	}
	return Data{Code: res.Code, Header: res.Header.Clone(), Body: body}, nil
}

func DoData(r Req) (Data, error) {
	res, err := Do(r)
	if err != nil {
		return Data{}, err
	}
	return Read(res, MaxBody)
}

func Get(url string) (Res, error) {
	return Do(Req{URL: url, Method: http.MethodGet})
}

func Post(url string, body io.Reader) (Res, error) {
	return Do(Req{URL: url, Method: http.MethodPost, Body: body})
}

func GetData(url string) (Data, error) {
	return DoData(Req{URL: url, Method: http.MethodGet})
}

func PostData(url string, body io.Reader) (Data, error) {
	return DoData(Req{URL: url, Method: http.MethodPost, Body: body})
}

func Multipart(r Req, fields []Field, files []File) (Res, error) {
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)

	go func() {
		err := writeMultipart(mw, fields, files)
		if cerr := mw.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			_ = pw.CloseWithError(err)
			return
		}
		_ = pw.Close()
	}()

	if r.Method == "" {
		r.Method = http.MethodPost
	}
	if r.Headers == nil {
		r.Headers = make(map[string]string)
	}
	r.Headers["Content-Type"] = mw.FormDataContentType()
	r.Body = pr

	res, err := Do(r)
	if err != nil {
		_ = pr.Close()
		return Res{}, err
	}
	return res, nil
}

func PostMultipart(url string, fields []Field, files []File) (Res, error) {
	return Multipart(Req{URL: url, Method: http.MethodPost}, fields, files)
}

func PostMultipartData(url string, fields []Field, files []File) (Data, error) {
	res, err := PostMultipart(url, fields, files)
	if err != nil {
		return Data{}, err
	}
	return Read(res, MaxBody)
}

func MultipartData(r Req, fields []Field, files []File) (Data, error) {
	res, err := Multipart(r, fields, files)
	if err != nil {
		return Data{}, err
	}
	return Read(res, MaxBody)
}

func writeMultipart(mw *multipart.Writer, fields []Field, files []File) error {
	for _, field := range fields {
		if field.Name == "" {
			return e.New("", e.Call, "web:multipart", "field_name")
		}
		if err := mw.WriteField(field.Name, field.Val); err != nil {
			return e.Wrap("", e.Trans, "web:multipart", err)
		}
	}

	for _, file := range files {
		if file.Field == "" || file.Path == "" {
			return e.New("", e.Call, "web:multipart", "file_arg")
		}
		name := file.Name
		if name == "" {
			name = filepath.Base(file.Path)
		}

		f, err := os.Open(file.Path)
		if err != nil {
			return e.Wrap("", e.Trans, "web:multipart", err)
		}
		part, err := createFilePart(mw, file.Field, name, file.MIME)
		if err != nil {
			f.Close()
			return err
		}
		_, err = io.Copy(part, f)
		cerr := f.Close()
		if err != nil {
			return e.Wrap("", e.Trans, "web:multipart", err)
		}
		if cerr != nil {
			return e.Wrap("", e.Trans, "web:multipart", cerr)
		}
	}
	return nil
}

func createFilePart(mw *multipart.Writer, field, name, mime string) (io.Writer, error) {
	if mime == "" {
		w, err := mw.CreateFormFile(field, name)
		if err != nil {
			return nil, e.Wrap("", e.Trans, "web:multipart", err)
		}
		return w, nil
	}
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition",
		`form-data; name="`+escapeQuotes(field)+`"; filename="`+escapeQuotes(name)+`"`)
	h.Set("Content-Type", mime)
	part, err := mw.CreatePart(h)
	if err != nil {
		return nil, e.Wrap("", e.Trans, "web:multipart", err)
	}
	return part, nil
}

func escapeQuotes(s string) string {
	return strings.NewReplacer("\\", "\\\\", `"`, "\\\"").Replace(s)
}
