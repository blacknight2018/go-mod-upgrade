package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	term "github.com/AlecAivazis/survey/v2/terminal"
	"github.com/Masterminds/semver/v3"
	"github.com/fatih/color"
	"golang.org/x/crypto/ssh/terminal"
)

func max(x, y int) int {
	if x > y {
		return x
	}
	return y
}

func padRight(str string, length int) string {
	if len(str) >= length {
		return str
	}
	return str + strings.Repeat(" ", length-len(str))
}

func formatName(module Module, length int) string {
	c := color.New(color.FgWhite).SprintFunc()
	from := module.from
	to := module.to
	if from.Minor() != to.Minor() {
		c = color.New(color.FgYellow).SprintFunc()
	}
	if from.Patch() != to.Patch() {
		c = color.New(color.FgGreen).SprintFunc()
	}
	if from.Prerelease() != to.Prerelease() {
		c = color.New(color.FgRed).SprintFunc()
	}
	return c(padRight(module.name, length))
}

func formatFrom(from *semver.Version, length int) string {
	c := color.New(color.FgBlue).SprintFunc()
	return c(padRight(from.String(), length))
}

func formatTo(module Module) string {
	green := color.New(color.FgGreen).SprintFunc()
	var buf bytes.Buffer
	from := module.from
	to := module.to
	same := true
	fmt.Fprintf(&buf, "%d.", to.Major())
	if from.Minor() == to.Minor() {
		fmt.Fprintf(&buf, "%d.", to.Minor())
	} else {
		fmt.Fprintf(&buf, "%s%s", green(to.Minor()), green("."))
		same = false
	}
	if from.Patch() == to.Patch() && same {
		fmt.Fprintf(&buf, "%d", to.Patch())
	} else {
		fmt.Fprintf(&buf, "%s", green(to.Patch()))
		same = false
	}
	if to.Prerelease() != "" {
		if from.Prerelease() == to.Prerelease() && same {
			fmt.Fprintf(&buf, "-%s", to.Prerelease())
		} else {
			fmt.Fprintf(&buf, "-%s", green(to.Prerelease()))
		}
	}
	if to.Metadata() != "" {
		fmt.Fprintf(&buf, "%s%s", green("+"), green(to.Metadata()))
	}
	return buf.String()
}

type Module struct {
	name string
	from *semver.Version
	to   *semver.Version
}

func discover(verbose bool) ([]Module, error) {
	fmt.Println("Discovering modules...")
	args := []string{
		"list",
		"-u",
		"-mod=mod",
		"-f",
		"'{{if (and (not (or .Main .Indirect)) .Update)}}{{.Path}}: {{.Version}} -> {{.Update.Version}}{{end}}'",
		"-m",
		"all",
	}
	list, err := exec.Command("go", args...).Output()
	if err != nil {
		return nil, err
	}
	split := strings.Split(string(list), "\n")
	modules := []Module{}
	re := regexp.MustCompile(`'(.+): (.+) -> (.+)'`)
	for _, x := range split {
		if x != "''" && x != "" {
			matched := re.FindStringSubmatch(x)
			if len(matched) < 4 {
				return nil, fmt.Errorf("Couldn't parse module %s", x)
			}
			name, from, to := matched[1], matched[2], matched[3]
			if verbose {
				fmt.Printf("Found module %s, from %s to %s\n", name, from, to)
			}
			fromversion, err := semver.NewVersion(from)
			if err != nil {
				return nil, err
			}
			toversion, err := semver.NewVersion(to)
			if err != nil {
				return nil, err
			}
			d := Module{
				name: name,
				from: fromversion,
				to:   toversion,
			}
			modules = append(modules, d)
		}
	}
	return modules, nil
}

func choose(modules []Module, pageSize int) []Module {
	maxName := 0
	maxFrom := 0
	maxTo := 0
	for _, x := range modules {
		maxName = max(maxName, len(x.name))
		maxFrom = max(maxFrom, len(x.from.String()))
		maxTo = max(maxTo, len(x.to.String()))
	}
	fd := int(os.Stdout.Fd())
	termWidth, _, err := terminal.GetSize(fd)
	if err != nil {
		fmt.Printf("Error while getting terminal size %v\n", err)
	}
	options := []string{}
	for _, x := range modules {
		from := ""
		// Only show from when the terminal width is big enough
		// As there is a bug in survey when the terminal overflows
		// https://github.com/AlecAivazis/survey/issues/101
		if termWidth > maxName+maxFrom+maxTo+11 {
			from = formatFrom(x.from, maxFrom)
		}
		options = append(options, fmt.Sprintf("%s %s -> %s", formatName(x, maxName), from, formatTo(x)))
	}
	prompt := &survey.MultiSelect{
		Message:  "Choose which modules to update",
		Options:  options,
		PageSize: pageSize,
	}
	choice := []int{}
	err = survey.AskOne(prompt, &choice)
	if err == term.InterruptErr {
		fmt.Println("Bye")
		os.Exit(0)
	} else if err != nil {
		log.Fatal(err)
	}
	updates := []Module{}
	for _, x := range choice {
		updates = append(updates, modules[x])
	}
	return updates
}

func update(modules []Module) {
	for _, x := range modules {
		fmt.Fprintf(color.Output, "Updating %s to version %s...\n", formatName(x, len(x.name)), formatTo(x))
		out, err := exec.Command("go", "get", x.name).CombinedOutput()
		if err != nil {
			fmt.Printf("Error while updating %s: %s\n", x.name, string(out))
		}
	}
}

func main() {
	var verbose bool
	var pageSize int
	flag.IntVar(&pageSize, "p", 10, "Specify page size, Default is 10")
	flag.BoolVar(&verbose, "v", false, "Verbose mode")
	flag.Parse()
	modules, err := discover(verbose)
	if err != nil {
		log.Fatal(err)
	}
	if len(modules) > 0 {
		modules = choose(modules, pageSize)
		update(modules)
	} else {
		fmt.Println("All modules are up to date")
	}
}
