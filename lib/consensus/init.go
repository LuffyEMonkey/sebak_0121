package consensus

import (
	logging "github.com/inconshreveable/log15"

	"boscoin.io/sebak/lib/common"
)

var log logging.Logger = logging.New("module", "consensus")

func init() {
	SetLogging(common.DefaultLogLevel, common.DefaultLogHandler)
}

func SetLogging(level logging.Lvl, handler logging.Handler) {
	log.SetHandler(logging.LvlFilterHandler(level, handler))
}
