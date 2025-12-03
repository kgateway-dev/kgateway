package downward

import (
	"bytes"
	"io"
	"text/template"
)

type Interpolator interface {
	InterpolateIO(in io.Reader, out io.Writer, data DownwardAPI) error
	Interpolate(tmpl string, out io.Writer, data DownwardAPI) error
	InterpolateString(*string, DownwardAPI) error
}

func NewInterpolator() Interpolator {
	return &interpolator{}
}

type interpolator struct{}

func (i *interpolator) InterpolateIO(in io.Reader, out io.Writer, data DownwardAPI) error {
	inbyte, err := io.ReadAll(in)
	if err != nil {
		return err
	}

	return i.Interpolate(string(inbyte), out, data)
}

func (*interpolator) Interpolate(tmpl string, out io.Writer, data DownwardAPI) error {
	t, err := template.New("template").Option("missingkey=zero").Parse(tmpl)
	if err != nil {
		return err
	}
	err = t.Execute(out, data)
	if err != nil {
		return err
	}
	return nil
}

func (i *interpolator) InterpolateString(tmpl *string, data DownwardAPI) error {
	var b bytes.Buffer
	err := i.Interpolate(*tmpl, &b, data)
	if err != nil {
		return err
	}
	*tmpl = b.String()
	return nil
}
