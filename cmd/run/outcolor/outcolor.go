package outcolor

import (
	"encoding/json"
	"github.com/TylerBrock/colorjson"
	"github.com/argoproj/dev-tools/cmd/run"
	"github.com/fatih/color"
	"os"
	"strings"
)

var (
	colorError        = color.New(color.FgRed)
	colorErrorSprintf = colorError.SprintFunc()
	colorWarn         = color.New(color.FgYellow)
	colorWarnSprintf  = colorWarn.SprintFunc()
)

func ColorizeGoreman(in string) *string {
	const sep = `{"`

	out := in
	// Colorize if json
	if before, after, found := strings.Cut(in, sep); found {
		after = sep + after
		var obj map[string]interface{}
		if json.Unmarshal([]byte(after), &obj) == nil {
			f := colorjson.NewFormatter()
			if level, ok := obj["level"]; ok {
				if level == "error" || level == "fatal" || level == "panic" {
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

	return ColorizeGoLog(out)
}

func ColorizeGoLog(str string) *string {
	run.Out(os.Stderr, "ColorizeGoLog: "+str)
	// Colorize levels in field output
	str = highlight(str, "level=error", colorErrorSprintf)
	str = highlight(str, "level=warning", colorWarnSprintf)
	return &str
}

func highlight(in string, search string, color func(a ...interface{}) string) string {
	return strings.ReplaceAll(in, search, color(search))
}
