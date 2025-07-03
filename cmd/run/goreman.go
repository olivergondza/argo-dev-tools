package main

import (
	"encoding/json"
	"github.com/TylerBrock/colorjson"
	"github.com/fatih/color"
	"strings"
)

var (
	colorError        = color.New(color.FgRed)
	colorErrorSprintf = colorError.SprintFunc()
	colorWarn         = color.New(color.FgYellow)
	colorWarnSprintf  = colorWarn.SprintFunc()
)

func osExecGoreman(args ...string) error {
	mp := newManagedProc(args...)
	mp.stdoutTransformer = stdoutTransformer
	if err := mp.run(); err != nil {
		return err
	}

	return nil
}

func stdoutTransformer(in string) string {
	const sep = `{"`

	out := in
	// Colorize if json
	if before, after, found := strings.Cut(in, sep); found {
		after = sep + after
		var obj map[string]interface{}
		if json.Unmarshal([]byte(after), &obj) == nil {
			f := colorjson.NewFormatter()
			if level, ok := obj["level"]; ok {
				if level == "error" {
					f.StringColor = colorError
				} else if level == "warning" {
					f.StringColor = colorWarn
				}
			}

			colorized, err := f.Marshal(obj)
			if err != nil {
				panic(err)
			}
			out = before + string(colorized)
		}
	}

	// Colorize levels in field output
	out = highlight(out, "level=error", colorErrorSprintf)
	out = highlight(out, "level=warning", colorWarnSprintf)

	return out
}

func highlight(in string, search string, color func(a ...interface{}) string) string {
	return strings.ReplaceAll(in, search, color(search))
}
