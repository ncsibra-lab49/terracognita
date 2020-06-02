package main

import (
	"fmt"
	"html/template"
	"io"
	"strings"

	"github.com/jinzhu/inflection"
	"github.com/pkg/errors"
)

const (
	// packageTmpl it's the package definition
	packageTmpl = `
	package reader

	// Code generated by github.com/cycloidio/terracognita/aws/cmd; DO NOT EDIT
	`

	// arTmpl it's the Reader interface template definition
	arTmpl = `
	// Reader is the interface defining all methods that need to be implemented
	//
	// The next behavior commented in the below paragraph, applies to every method
	// which clearly match what's explained, for the sake of not repeating the same,
	// over and over.
	// The most of the methods defined by this interface, return their results in a
	// map. Those maps, have as keys, the AWS region which have been requested and
	// the values are the items returned by AWS for such region.
	// Because the methods may make calls to different regions, in case that there
	// is an error on a region, the returned map won't have any entry for such
	// region and such errors will be reported by the returned error, nonetheless
	// the items, got from the successful requests to other regions, will be
	// returned, with the meaning that the methods will return partial results, in
	// case of errors.
	// For avoiding by the callers the problem of if the returned map may be nil,
	// the function will always return a map instance, which will be of length 0
	// in case that there is not any successful request.
	type Reader interface {
		// GetAccountID returns the current ID for the account used
		GetAccountID() string

		// GetRegion returns the currently used region for the Connector
		GetRegion() string

		{{ range . }}
			{{ .Documentation -}}
			{{ .Signature }}
		{{ end }}
	}
	`

	// functionTmpl it's the implementation of a function
	functionTmpl = `
		func (c *connector) {{ .Signature }} {
			{{ if ne .FilterByOwner ""}}
				if input == nil {
					input = &{{.Input}}{}
				}
				input.{{.FilterByOwner}} = append(input.{{.FilterByOwner}}, c.accountID)
			{{ end -}}

			if c.svc.{{.Service}} == nil {
				c.svc.{{.Service}} = {{.Service}}.New(c.svc.session)
			}

			{{ if .HasNoSlice }}
				var opt {{ .Output }}
			{{ else }}
				opt := make({{ .Output }}, 0)
			{{ end }}

			hasNextToken := true
			for hasNextToken {
				o, err := c.svc.{{.Service}}.{{.ServiceEntityFn}}WithContext(ctx, input)
				if err != nil {
					return nil, err
				}
				{{ if .HasNotPagination }}
					hasNextToken = false
				{{ else }}
					if input == nil {
						input = &{{.Input}}{}
					}
					input.{{.InputPaginationAttributeFn}} = o.{{.PaginationAttributeFn}}
					hasNextToken = o.{{.PaginationAttributeFn}} != nil
				{{ end }}

				{{ if .IsAttributeListSlice }}
					for _,v := range o.{{ index .AttributeList 0 }} {
						opt = append(opt, v.{{ index .AttributeList 1 }}...)
					}
				{{ else if .HasNoSlice }}
					opt = o.{{ index .AttributeList 0 }}
				{{ else if .IsMap }}
					opt = o.{{ index .AttributeList 0 }}
				{{ else }}
					opt = append(opt, o.{{ index .AttributeList 0 }}...)
				{{ end }}
			}

			return opt, nil
		}
	`
)

var (
	fnTmpl        *template.Template
	pkgTmpl       *template.Template
	awsReaderTmpl *template.Template
)

func init() {
	var err error

	fnTmpl, err = template.New("test").Parse(functionTmpl)
	if err != nil {
		panic(err)
	}

	pkgTmpl, err = template.New("test").Parse(packageTmpl)
	if err != nil {
		panic(err)
	}

	awsReaderTmpl, err = template.New("test").Parse(arTmpl)
	if err != nil {
		panic(err)
	}
}

// Function is the definition of one of the functions
type Function struct {
	// FnName is the name of the function
	// if not defined "Get{{.Entity}i" is used
	FnName string

	// Entity is the name of the entity, like
	// CloudFrontOriginAccessIdentities, Instances etc
	Entity string

	// FnAttributeList defines the attribute inside of the output
	// that holds all the resources to return
	// If defined like 'attribute.name' it'll call that directly
	// If defined like 'attribute#name' it'll iterate over 'attribute'
	// and 'name' will be used ad the list item to fetch
	FnAttributeList string

	// Some functions on AWS have the "Describe" prefix
	// or the "List" prefix, so it has to be specified
	// which one to use
	Prefix string

	// Service is the AWS service that it uses, basically the
	// pkg name, so "ec2", "cloudfront" etc
	Service string

	// FnServiceEntity is the name of the Entity function to use on the Service
	FnServiceEntity string

	// Documentation is the documentation that will be added
	// to the AWSReader function definition, as it's the
	// only public part that could be seen on the godocs
	Documentation string

	// Is the Output name that it has
	FnOutput string

	// FnSignature is the signture it has to be used on the Interface
	// AWSReader and the function implementation
	FnSignature string

	// NoGenerateFn avoids generating the function implementation as
	// it's to different from the templates we use
	// If true, it should be used with 'Signature' to add it to the
	// AWSReader and have the custom implementation outside of the
	// generated code
	NoGenerateFn bool

	// FilterByOwner adds the "{{.FilterByOwner}} = AccountID" to the input filter
	// so this value has to be the correct name on the input
	FilterByOwner string

	// HasNotPagination flags if the resource has NextToken logic or not
	HasNotPagination bool

	// HasNoSlice means that it's not an [] to return but a single item
	HasNoSlice bool

	// FnPaginationAttribute overrides the default name NextToken
	FnPaginationAttribute string

	// FnInputPaginationAttribute overrides the default reciever of the
	// pagination name FnPaginationAttribute
	FnInputPaginationAttribute string

	// SingularEntity represents the singular value of an entity
	SingularEntity string

	// If the value is a map
	IsMap bool
}

// Name builds a name simply using "Get{{.Entity}}"
// except if FnName is defined, in which case
// only FnName is used
func (f Function) Name() string {
	if f.FnName != "" {
		return f.FnName
	}

	prefix := "Get"
	if f.FilterByOwner != "" {
		prefix += "Own"
	}

	return fmt.Sprintf("%s%s", prefix, f.Entity)
}

// Output builds the output by "{{.Service}}.{{singular(.Entity)}}"
// except if FnOutput is defined in which case the formula
// "{{.FnOutput}}" is used
func (f Function) Output() string {
	var typePrefix = "[]*"
	if f.IsMap {
		typePrefix = "map[string]*"
	}
	if f.HasNoSlice {
		typePrefix = "*"
	}
	if f.FnOutput != "" {
		return fmt.Sprintf("%s%s", typePrefix, f.FnOutput)
	}

	if f.SingularEntity != "" {
		return fmt.Sprintf("%s%s.%s", typePrefix, f.Service, f.SingularEntity)
	}
	return fmt.Sprintf("%s%s.%s", typePrefix, f.Service, inflection.Singular(f.Entity))
}

// Input builds the input by "{{.Service}}.{{.Prefix}}{{.Entity}}"
func (f Function) Input() string {
	return fmt.Sprintf("%s.%sInput", f.Service, f.ServiceEntityFn())
}

// Signature builds the signature except if FnSignature it's defined,
// in which case is used
func (f Function) Signature() string {
	if f.FnSignature != "" {
		return f.FnSignature
	}

	return fmt.Sprintf("%s (ctx context.Context, input *%s) (%s, error)", f.Name(), f.Input(), f.Output())
}

// AttributeList returns all the list of attributes to access to get the value
// that we want, if it has the '#' it means it's inside of an array, if not
// its used as a simple access.
func (f Function) AttributeList() []string {
	if f.FnAttributeList != "" {
		if f.IsAttributeListSlice() {
			return strings.Split(f.FnAttributeList, "#")
		}
		return []string{f.FnAttributeList}
	}

	return []string{f.Entity}
}

// IsAttributeListSlice checks if the logic should be to
// access an attribute or to iterate over attributes
func (f Function) IsAttributeListSlice() bool {
	if strings.Contains(f.FnAttributeList, "#") {
		return true
	}
	return false
}

// ServiceEntityFn the name of the function to call on the
// service to get the Entity
func (f Function) ServiceEntityFn() string {
	if f.FnServiceEntity != "" {
		return fmt.Sprintf("%s%s", f.Prefix, f.FnServiceEntity)
	}

	return fmt.Sprintf("%s%s", f.Prefix, f.Entity)

}

// PaginationAttributeFn is the attribute that defined the Pagination
func (f Function) PaginationAttributeFn() string {
	if f.FnPaginationAttribute != "" {
		return f.FnPaginationAttribute
	}

	return "NextToken"
}

// InputPaginationAttributeFn is the attribute that defines the
// pagination on the input filter
func (f Function) InputPaginationAttributeFn() string {
	if f.FnInputPaginationAttribute != "" {
		return f.FnInputPaginationAttribute
	}

	return f.PaginationAttributeFn()
}

// Execute uses the fnTmpl to interpolate f
// and write the result to w
func (f Function) Execute(w io.Writer) error {
	if f.NoGenerateFn {
		return nil
	}

	err := fnTmpl.Execute(w, f)
	if err != nil {
		return errors.Wrapf(err, "failed to Execute with Function %+v", f)
	}

	return nil
}
