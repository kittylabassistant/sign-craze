package dpi

import (
	"context"
	"fmt"

	"github.com/kittylabassistant/sign-craze/internal/exectx"
	"github.com/kittylabassistant/sign-craze/internal/firewall"
	"github.com/kittylabassistant/sign-craze/internal/log"
)

// NFQueue управляет правилами NFQUEUE в цепочке signcraze_dpi.
type NFQueue struct {
	ipt *firewall.IPTables
}

// NewNFQueue создаёт NFQueue с заданным runner.
func NewNFQueue(runner exectx.Runner) *NFQueue {
	return &NFQueue{ipt: firewall.New(runner)}
}

// Enable добавляет правила NFQUEUE в цепочку mangle/signcraze_dpi.
// Идемпотентно. Применяется при `--dpi on` и переключении в режим hybrid.
func (q *NFQueue) Enable(ctx context.Context, queueNum int, fwmark uint32) error {
	log.L().Info("dpi: включение NFQUEUE", "queue", queueNum)

	mark := fmt.Sprintf("0x%x", fwmark)
	queue := fmt.Sprintf("%d", queueNum)

	rules := [][]string{
		{
			"-m", "mark", "!", "--mark", mark,
			"-p", "tcp",
			"-j", "NFQUEUE", "--queue-num", queue, "--queue-bypass",
			"-m", "comment", "--comment", "signcraze:dpi-tcp",
		},
		{
			"-m", "mark", "!", "--mark", mark,
			"-p", "udp",
			"-j", "NFQUEUE", "--queue-num", queue, "--queue-bypass",
			"-m", "comment", "--comment", "signcraze:dpi-udp",
		},
	}

	for _, rule := range rules {
		if err := q.ipt.EnsureRule(ctx, "mangle", "signcraze_dpi", rule...); err != nil {
			return fmt.Errorf("dpi nfqueue: добавление правила: %w", err)
		}
	}

	log.L().Info("dpi: NFQUEUE включён")
	return nil
}

// Disable удаляет правила NFQUEUE из цепочки mangle/signcraze_dpi.
// Идемпотентно. Применяется при `--dpi off`.
func (q *NFQueue) Disable(ctx context.Context, queueNum int, fwmark uint32) error {
	log.L().Info("dpi: отключение NFQUEUE", "queue", queueNum)

	mark := fmt.Sprintf("0x%x", fwmark)
	queue := fmt.Sprintf("%d", queueNum)

	rules := [][]string{
		{
			"-m", "mark", "!", "--mark", mark,
			"-p", "tcp",
			"-j", "NFQUEUE", "--queue-num", queue, "--queue-bypass",
			"-m", "comment", "--comment", "signcraze:dpi-tcp",
		},
		{
			"-m", "mark", "!", "--mark", mark,
			"-p", "udp",
			"-j", "NFQUEUE", "--queue-num", queue, "--queue-bypass",
			"-m", "comment", "--comment", "signcraze:dpi-udp",
		},
	}

	for _, rule := range rules {
		if err := q.ipt.DeleteRule(ctx, "mangle", "signcraze_dpi", rule...); err != nil {
			return fmt.Errorf("dpi nfqueue: удаление правила: %w", err)
		}
	}

	log.L().Info("dpi: NFQUEUE отключён")
	return nil
}
