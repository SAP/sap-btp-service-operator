package sm

import (
	"net/url"
	"strings"
)

// Parameters holds common query parameters
type Parameters struct {
	FieldQuery    []string
	LabelQuery    []string
	GeneralParams []string
}

const (
	fieldQuery = "fieldQuery"
	labelQuery = "labelQuery"
)

// Encode encodes the parameters as URL query parameters
func (p *Parameters) Encode() string {
	if p == nil {
		return ""
	}

	v := url.Values{}

	if len(p.FieldQuery) > 0 {
		v.Set(fieldQuery, strings.Join(p.FieldQuery, " and "))
	}

	if len(p.LabelQuery) > 0 {
		v.Set(labelQuery, strings.Join(p.LabelQuery, " and "))
	}

	for _, param := range p.GeneralParams {
		s := strings.SplitN(param, "=", 2)
		if len(s) < 2 {
			v.Add(param, "")
		} else {
			v.Add(s[0], s[1])
		}
	}

	return v.Encode()
}
