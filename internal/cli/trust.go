package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/aaroncutress/kmp-scaffold/internal/scaffold/filetmpl"
)

// installTrustPrompt is how the user is asked about a remote template the first
// time a given commit of it is used.
//
// What is shown is deliberately concrete: which repository, which commit, and
// how much it would do. The answer is about that commit, not the repository, so
// the same template at a later commit is asked about again.
func installTrustPrompt() {
	filetmpl.SetTrustPrompt(func(t filetmpl.Trust) (bool, error) {
		if !interactive() {
			return false, fmt.Errorf(
				"%s has not been used before, and there is no terminal to ask on - "+
					"run it once interactively to review it, or pass --trust", t.Ref)
		}

		fmt.Println()
		fmt.Println(sBold.Render("A template from the internet"))
		fmt.Println()

		row := func(label, value string) {
			fmt.Printf("  %-10s %s\n", label, value)
		}
		row("Template", t.Label+sMuted.Render("  ("+t.ID+")"))
		row("From", t.URL)
		row("Commit", t.Revision)

		writes := fmt.Sprintf("%d file(s)", t.Files)
		if t.Edits > 0 {
			writes += fmt.Sprintf(", and inserts into existing ones in %d place(s)", t.Edits)
		}
		row("Writes", writes)
		if len(t.Recipes) > 0 {
			row("Can add", strings.Join(t.Recipes, ", "))
		}
		row("Runs", sMuted.Render("nothing - templates cannot execute commands"))

		fmt.Println()
		fmt.Println(sMuted.Render("  Review it at " + t.URL))
		fmt.Println(sMuted.Render("  Saying yes remembers this commit; a later one asks again."))
		fmt.Println()
		fmt.Print(sBold.Render("Use this template? ") + sMuted.Render("[y/N] "))

		answer, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil {
			return false, nil
		}
		switch strings.ToLower(strings.TrimSpace(answer)) {
		case "y", "yes":
			return true, nil
		default:
			return false, nil
		}
	})
}
