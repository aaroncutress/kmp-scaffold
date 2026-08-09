package tui

import (
	"fmt"
	"strings"

	"github.com/aaroncutress/kmp-scaffold/internal/scaffold"
)

// StepsFor turns a template's questions into wizard screens.
func StepsFor(questions []scaffold.Question) []Step {
	steps := make([]Step, 0, len(questions))
	for _, q := range questions {
		steps = append(steps, StepFor(q))
	}
	return steps
}

// StepFor turns one question into the screen that answers it.
//
// Every step records the answer under the question's id and then calls the
// question's own Apply, so a template that keeps a typed view of the answers
// gets to update it without having to reimplement the storing.
func StepFor(q scaffold.Question) Step {
	switch q.Kind {
	case scaffold.KindSelect:
		return &SelectStep{
			Prompt:  q.Prompt,
			Hint:    q.Hint,
			SkipIf:  skipFor(q),
			Options: q.OptionList,
			Current: func(a *scaffold.Answers) string {
				if a.Has(q.ID) {
					return a.Str(q.ID)
				}
				return asString(q.DefaultValue(a))
			},
			Apply: func(a *scaffold.Answers, id string) { record(q, a, id) },
		}

	case scaffold.KindMultiSelect:
		return &CheckStep{
			Prompt:     q.Prompt,
			Hint:       q.Hint,
			AllowEmpty: q.AllowEmpty,
			SkipIf:     skipFor(q),
			Options:    q.OptionList,
			Current: func(a *scaffold.Answers) []string {
				if a.Has(q.ID) {
					return a.Strs(q.ID)
				}
				return asStrings(q.DefaultValue(a))
			},
			Apply: func(a *scaffold.Answers, ids []string) { record(q, a, ids) },
		}

	case scaffold.KindConfirm:
		def := false
		if b, ok := q.Default.(bool); ok {
			def = b
		}
		return &ConfirmStep{
			Prompt:  q.Prompt,
			Hint:    q.Hint,
			Default: def,
			SkipIf:  skipFor(q),
			Apply:   func(a *scaffold.Answers, v bool) { record(q, a, v) },
		}

	case scaffold.KindList:
		// A list is typed as one comma-separated line, which is far less
		// tedious than a checklist of things that do not exist yet.
		return &TextStep{
			Prompt: q.Prompt,
			Hint:   q.Hint,
			SkipIf: skipFor(q),
			Default: func(a *scaffold.Answers) string {
				if a.Has(q.ID) {
					return strings.Join(a.Strs(q.ID), ", ")
				}
				return strings.Join(asStrings(q.DefaultValue(a)), ", ")
			},
			Validate: func(v string, a *scaffold.Answers) error {
				return validate(q, splitList(v), a)
			},
			Apply: func(a *scaffold.Answers, v string) { record(q, a, splitList(v)) },
		}

	default:
		return &TextStep{
			Prompt: q.Prompt,
			Hint:   q.Hint,
			SkipIf: skipFor(q),
			Default: func(a *scaffold.Answers) string {
				if a.Has(q.ID) {
					return a.Str(q.ID)
				}
				return asString(q.DefaultValue(a))
			},
			Validate: func(v string, a *scaffold.Answers) error { return validate(q, v, a) },
			Apply:    func(a *scaffold.Answers, v string) { record(q, a, v) },
		}
	}
}

func skipFor(q scaffold.Question) func(*scaffold.Answers) bool {
	if q.SkipFor == nil {
		return nil
	}
	return q.SkipFor
}

func validate(q scaffold.Question, v any, a *scaffold.Answers) error {
	if q.Validate == nil {
		return nil
	}
	return q.Validate(v, a)
}

func record(q scaffold.Question, a *scaffold.Answers, v any) {
	a.Set(q.ID, v)
	if q.Apply != nil {
		q.Apply(a, v)
	}
}

// splitList parses a comma-separated answer, dropping empties so a trailing
// comma is not an error.
func splitList(v string) []string {
	var out []string
	for _, part := range strings.Split(v, ",") {
		if t := strings.TrimSpace(part); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func asString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	default:
		return fmt.Sprint(t)
	}
}

func asStrings(v any) []string {
	switch t := v.(type) {
	case nil:
		return nil
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			out = append(out, asString(item))
		}
		return out
	case string:
		return splitList(t)
	default:
		return nil
	}
}
