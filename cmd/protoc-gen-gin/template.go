package main

import (
	"bytes"
	"strings"
	"text/template"
)

var httpTemplate = `
{{$svrType := .ServiceType}}

type {{.ServiceType}}HTTPServer interface {
{{- range .MethodSets}}
	{{.Name}}(context.Context, *{{.Request}}) (*{{.Reply}}, error)
{{- end}}
}

func Register{{.ServiceType}}HTTPServer(srv {{.ServiceType}}HTTPServer) func(gin.IRouter) {
	return func(group gin.IRouter){
	{{- range .Methods}}
	group.{{.Method}}("{{.Path}}", _{{$svrType}}_{{.Name}}{{.Num}}_HTTP_Handler(srv))
	{{- end}}
	}
}

{{range .Methods}}
func _{{$svrType}}_{{.Name}}{{.Num}}_HTTP_Handler(srv {{$svrType}}HTTPServer) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		var (
			in {{.Request}}
			out *{{.Reply}}
			err error
			newCtx context.Context
		)

		{{- if .HasParam}}
		err = ctx.ShouldBindUri(&in)
		{{- else}}
		err = ctx.ShouldBind(&in)
		{{- end}}
		if err != nil {
			err = errors.FormatInvalid(errors.InvalidFormat, err.Error())
			goto Label
		}

		err = in.ValidateAll()
		if err != nil {
			err = errors.ParamInvalid(errors.InvalidParam, err.Error())
			goto Label
		}

		newCtx = context.WithValue(ctx.Request.Context(), pkg.Token, ctx.GetHeader(pkg.Token))
		out, err = srv.{{.Name}}(newCtx, &in)

Label:
	http.Result(out, err)(ctx)
	}
}
{{end}}
`

type serviceDesc struct {
	PackageName string
	ServiceType string // Greeter
	ServiceName string // helloworld.Greeter
	Metadata    string // api/helloworld/helloworld.proto
	Methods     []*methodDesc
	MethodSets  map[string]*methodDesc
}

type methodDesc struct {
	// method
	Name         string
	OriginalName string // The parsed original name
	Num          int
	Request      string
	Reply        string
	// http_rule
	Path         string
	Method       string
	HasParam     bool
	HasBody      bool
	Body         string
	ResponseBody string
}

func (s *serviceDesc) execute() string {
	s.MethodSets = make(map[string]*methodDesc)
	for _, m := range s.Methods {
		s.MethodSets[m.Name] = m
	}
	buf := new(bytes.Buffer)
	tmpl, err := template.New("http").Parse(strings.TrimSpace(httpTemplate))
	if err != nil {
		panic(err)
	}
	if err := tmpl.Execute(buf, s); err != nil {
		panic(err)
	}
	return strings.Trim(buf.String(), "\r\n")
}
