package service

import (
	"time"

	"github.com/bobziuchkovski/cue"
	"github.com/bobziuchkovski/cue/collector"
	"github.com/bobziuchkovski/cue/format"
	env "github.com/remerge/go-env"
)

func setLogFormat() {
	formatter := format.Formatf(
		"%v [%v:%v] %v",
		format.Level,
		format.ContextName,
		format.SourceWithLine,
		format.HumanMessage,
	)

	if !env.IsProd() {
		formatter = format.Colorize(
			format.Formatf(
				"%v %v",
				format.Time(time.RFC3339),
				formatter,
			),
		)
	}

	cue.Collect(cue.DEBUG, collector.Terminal{
		Formatter: formatter,
	}.New())
}
