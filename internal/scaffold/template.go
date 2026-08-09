// Package scaffold defines what a template is.
//
// A template owns a whole generatable project: the questions it asks, the files
// it writes, and - through the feature interface below - the things it can add
// to a project later. The tool itself only knows about the universal parts (a
// project's name, directory and package, and the list of features it has), so
// adding a template does not mean changing the wizard, the CLI or the manifest.
//
// Two kinds of template exist. Built-in ones are Go code and can do anything:
// compute options from a registry, run compatibility rules over resolved
// versions, fetch a binary. File-based ones are a directory of Go templates
// with a manifest, and trade that power for being writable without a compiler.
// Both satisfy Template, so nothing downstream can tell them apart.
//
// This package must not import the packages that implement templates. Built-in
// templates register themselves from their own init(), the same way generators
// already do.
package scaffold

import (
	"context"

	"github.com/aaroncutress/kmp-scaffold/internal/model"
	"github.com/aaroncutress/kmp-scaffold/internal/render"
	"github.com/aaroncutress/kmp-scaffold/internal/resolve"
	"github.com/aaroncutress/kmp-scaffold/internal/wire"
)

// SourceKind is where a template was loaded from.
type SourceKind string

const (
	// SourceBuiltin is compiled into the binary.
	SourceBuiltin SourceKind = "builtin"
	// SourcePath is a directory given on the command line.
	SourcePath SourceKind = "path"
	// SourceUser is a directory in the user's templates folder.
	SourceUser SourceKind = "user"
	// SourceRemote was fetched from a git remote.
	SourceRemote SourceKind = "remote"
)

// Source records where a template came from, so a project can load the same one
// again when a feature is added to it later.
type Source struct {
	Kind SourceKind
	// Ref is what the user typed: a path, or "github:owner/repo@v2".
	Ref string
	// Revision is the resolved commit for a remote template.
	Revision string
}

// String is the form recorded in the project manifest.
func (s Source) String() string {
	if s.Kind == SourceBuiltin || s.Ref == "" {
		return string(SourceBuiltin)
	}
	return s.Ref
}

// Meta describes a template to the CLI and the wizard.
type Meta struct {
	ID          string // "kmp-mobile"
	Label       string // "Kotlin Multiplatform app"
	Description string
	// Version is the template's own version, recorded in generated projects.
	Version string
	Source  Source

	// AsksPackage adds the universal "package name?" question. A template that
	// generates something without a package namespace leaves it off.
	AsksPackage bool

	// Sentinels are the files whose presence means a directory already holds a
	// project of this kind, so generating into it needs --force.
	Sentinels []string
}

// Ref projects the metadata into what the project manifest records.
func (m Meta) Ref() model.TemplateRef {
	return model.TemplateRef{
		ID:       m.ID,
		Source:   m.Source.String(),
		Revision: m.Source.Revision,
		Version:  m.Version,
	}
}

// Template is what `kmp-scaffold new` drives.
type Template interface {
	// Meta describes this template.
	Meta() Meta

	// NewAnswers returns an answer bag seeded with the template's defaults.
	NewAnswers() *Answers

	// Questions are asked in order, after the universal ones.
	Questions() []Question

	// Normalise repairs cross-question dependencies once every answer is in,
	// returning a note per change so the user is told what was adjusted.
	Normalise(*Answers) []string

	// Versions describes the resolution this template wants. A request with no
	// keys skips the resolve step altogether.
	Versions(*Answers) resolve.Request

	// Check adds template-specific findings to a finished resolution - the
	// compatibility warnings that are about this project rather than about the
	// versions themselves.
	Check(*Answers, *resolve.Result)

	// Headlines are the few versions worth putting on screen once resolution
	// finishes. The full set goes into whatever catalog the template generates.
	Headlines(*Answers, *resolve.Result) []Headline

	// Summary drives the review screen and the non-interactive printout.
	Summary(*Answers, *resolve.Result) []Section

	// Generate writes a whole project.
	Generate(context.Context, GenRequest) (*Report, error)

	// Vars is the template's own state, persisted into the project manifest and
	// read back by a later `add`. It must round-trip through encoding/json.
	Vars(*Answers) any

	// NextSteps is what to print after generating.
	NextSteps(*Answers) []NextStep
}

// FeatureTemplate is implemented by templates that can extend a project they
// generated. A template without it is not broken - `add feature` just says so
// rather than half-generating something.
//
// This is the first of what will become named recipes; the shape is kept narrow
// on purpose until there is a second one.
type FeatureTemplate interface {
	Template

	// FeatureNoun is what this template calls the thing being added, so the
	// prompts read naturally: "feature", "route", "screen".
	FeatureNoun() string

	// FeatureQuestions are asked after the universal name question.
	FeatureQuestions(*model.Manifest) []Question

	// FeatureSummary drives the review screen.
	FeatureSummary(*model.Manifest, *Answers) []Section

	// AddFeature writes the new files and wires them into the existing ones.
	AddFeature(context.Context, FeatureRequest) (*Report, error)
}

// GenRequest is everything Generate needs.
type GenRequest struct {
	Answers *Answers
	Result  *resolve.Result
	Writer  *render.Writer
	// Version is the kmp-scaffold version, stamped into generated headers.
	Version string
}

// FeatureRequest is everything AddFeature needs.
type FeatureRequest struct {
	// Manifest is the project's, already loaded. AddFeature updates and saves it.
	Manifest *model.Manifest
	Root     string
	// Name is the feature's name, validated as kebab-case by the tool.
	Name    string
	Answers *Answers
	Writer  *render.Writer
	Version string
	DryRun  bool
}

// Report summarises what a generation run did.
type Report struct {
	Writer   *render.Writer
	Wire     []wire.Result
	Warnings []string
	Manifest model.Manifest
}

// Section is one block of the review screen.
type Section struct {
	// Title is the block heading. An empty title runs the rows on from the
	// previous block.
	Title string
	// Rows are label/value pairs, rendered as an aligned table.
	Rows []Row
	// Items are rendered as a bulleted list. "(none)" is shown when empty.
	Items []string
	// Note is shown in the warning style, for something that needs attention.
	Note string
}

// Row is one label/value pair of a summary section.
type Row struct{ Label, Value string }

// Headline is one line of the resolved-versions table: what it is, what was
// chosen, and where that came from.
type Headline struct{ Label, Value, Note string }

// NextStep is one line of the "what to do now" list printed after generating.
type NextStep struct {
	// Command is printed as-is, indented, ready to paste.
	Command string
	// Note is a dimmed comment after it.
	Note string
}
