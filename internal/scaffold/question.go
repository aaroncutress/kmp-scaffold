package scaffold

import (
	"fmt"
	"strconv"

	"github.com/aaroncutress/kmp-scaffold/internal/model"
)

// Kind is the shape of a question, and so which wizard screen answers it.
type Kind string

const (
	// KindText is a single line of text.
	KindText Kind = "text"
	// KindList is a comma-separated line, answered as a []string.
	KindList Kind = "list"
	// KindSelect picks exactly one option.
	KindSelect Kind = "select"
	// KindMultiSelect is a checklist.
	KindMultiSelect Kind = "multiselect"
	// KindConfirm is yes/no.
	KindConfirm Kind = "confirm"
)

// Option is one choice in a select or checklist question.
type Option struct {
	ID    string
	Label string
	Desc  string
	// Disabled options are shown greyed out and cannot be chosen.
	Disabled bool
	// DisabledNote explains why, and replaces Desc when set.
	DisabledNote string
}

// Question is one thing a template asks.
//
// It has two halves. The declarative fields are what a file-based template sets
// and are enough for most questions. The hooks below them are Go-only, and win
// where both are given: they are how a built-in template computes options from
// a registry, or validates an answer against the rest of the project.
type Question struct {
	// ID is the stable key this answer is stored under.
	ID     string
	Kind   Kind
	Prompt string
	Hint   string

	// Default is the pre-filled answer: a string, []string or bool to match Kind.
	Default any
	// Options are the static choices for a select or checklist.
	Options []Option
	// AllowEmpty permits confirming a checklist with nothing ticked.
	AllowEmpty bool

	// DefaultFor computes the default from the answers so far.
	DefaultFor func(*Answers) any
	// OptionsFor computes the choices from the answers so far, which is what
	// lets a newly registered layout appear without touching the wizard.
	OptionsFor func(*Answers) []Option
	// SkipFor hides the question when earlier answers make it irrelevant.
	SkipFor func(*Answers) bool
	// Validate rejects an answer, with a message the user sees.
	Validate func(any, *Answers) error
	// Apply writes the answer somewhere other than Values[ID]. It runs after
	// the value has been stored, so it adds to the default behaviour rather
	// than replacing it.
	Apply func(*Answers, any)
}

// DefaultValue is the pre-filled answer for the current state.
func (q Question) DefaultValue(a *Answers) any {
	if q.DefaultFor != nil {
		return q.DefaultFor(a)
	}
	return q.Default
}

// OptionList is the choices for the current state.
func (q Question) OptionList(a *Answers) []Option {
	if q.OptionsFor != nil {
		return q.OptionsFor(a)
	}
	return q.Options
}

// Skip reports whether this question is irrelevant given the answers so far.
func (q Question) Skip(a *Answers) bool {
	return q.SkipFor != nil && q.SkipFor(a)
}

// ---------------------------------------------------------------------------
// Answers
// ---------------------------------------------------------------------------

// Answers is a template's answer bag.
//
// Project holds what the tool always asks for. Values holds one entry per
// question, and is what gets persisted, so it must stay JSON-safe. State is the
// template's own typed view of the same thing: a built-in template keeps a
// pointer to its real spec there and lets its question hooks work with that
// directly, rather than reading everything back out of a map.
type Answers struct {
	Project model.Project
	Values  map[string]any

	// Offline suppresses every network lookup. It is a property of this run
	// rather than an answer, so it is never persisted.
	Offline bool

	// InitGit asks for a git repository once the project exists. Also a
	// property of this run rather than of the project.
	InitGit bool

	state any
}

// NewAnswers returns an empty bag.
func NewAnswers() *Answers {
	return &Answers{Values: map[string]any{}}
}

// State returns the template's typed view.
func (a *Answers) State() any { return a.state }

// SetState stores the template's typed view.
func (a *Answers) SetState(v any) { a.state = v }

// Set records an answer.
func (a *Answers) Set(id string, v any) {
	if a.Values == nil {
		a.Values = map[string]any{}
	}
	a.Values[id] = v
}

// Has reports whether a question has been answered.
func (a *Answers) Has(id string) bool {
	_, ok := a.Values[id]
	return ok
}

// Str reads a text answer.
func (a *Answers) Str(id string) string {
	switch v := a.Values[id].(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	case nil:
		return ""
	default:
		return fmt.Sprint(v)
	}
}

// Strs reads a list answer.
//
// The []any case is not defensive programming: any value that has been through
// the manifest, or through a saved set of answers, comes back from encoding/json
// as []any, and a template that only handled []string would silently see an
// empty list.
func (a *Answers) Strs(id string) []string {
	switch v := a.Values[id].(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			} else {
				out = append(out, fmt.Sprint(item))
			}
		}
		return out
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	default:
		return nil
	}
}

// Bool reads a yes/no answer.
func (a *Answers) Bool(id string) bool {
	switch v := a.Values[id].(type) {
	case bool:
		return v
	case string:
		b, _ := strconv.ParseBool(v)
		return b
	default:
		return false
	}
}

// Int reads a numeric answer. JSON turns every number into a float64, so both
// forms have to be understood.
func (a *Answers) Int(id string) int {
	switch v := a.Values[id].(type) {
	case int:
		return v
	case float64:
		return int(v)
	case string:
		n, _ := strconv.Atoi(v)
		return n
	default:
		return 0
	}
}
