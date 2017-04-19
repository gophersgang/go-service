package service

import (
	"time"

	"github.com/bobziuchkovski/cue"
	"github.com/bobziuchkovski/cue/collector"
	"github.com/bobziuchkovski/cue/format"
	env "github.com/remerge/go-env"
)

func setLogFormat() {
	level := cue.INFO

	formatter := format.Formatf(
		"%v [%v:%v] %v",
		format.Level,
		format.ContextName,
		format.SourceWithLine,
		format.HumanMessage,
	)

	if !env.IsProd() {
		level = cue.DEBUG
		formatter = format.Colorize(
			format.Formatf(
				"%v %v",
				format.Time(time.RFC3339),
				formatter,
			),
		)
	}

	cue.Collect(level, collector.Terminal{
		Formatter: formatter,
	}.New())
}
