// Пакет geo: header-валидатор mihomo .mrs (rule-set) — только диагностика.
// Загрузчик для .mrs НЕ реализуется: mihomo качает свои rule-providers
// самостоятельно (см. internal/core/mihomo/render_rules.go — provider'ы
// формируются как `type: http` c `path: ./ruleset/<tag>.mrs`, mihomo
// разрешает URL и кеширует файл сам). Здесь нужен только pre-flight/diag
// валидатор — убедиться, что файл на диске похож на настоящий .mrs, а не на
// пустышку/HTML error-страницу/оборванную загрузку.
//
// Публичной спецификации формата .mrs байт-в-байт (уровня RFC/wiki с точной
// раскладкой полей) НЕТ. Раскладка ниже восстановлена ЭМПИРИЧЕСКИ: изучены
// ТОЛЬКО скомпилированные бинарные артефакты — реальные .mrs из релиза
// MetaCubeX/meta-rules-dat (ветка meta, июль 2026), БЕЗ обращения к исходному
// коду mihomo/meta-rules-dat (clean-room; mihomo — GPL-3.0, копировать код
// нельзя, а факт байтового формата — не код и не защищён копирайтом). Три
// независимых образца (см. также tasks для этой реализации):
//   - geo/geosite/youtube.mrs (behavior=domain,  count=178)
//   - geo/geosite/discord.mrs (behavior=domain,  count=28)
//   - geo/geoip/ru.mrs       (behavior=ipcidr,  count=47457)
//
// ВАЖНО (подводный камень): файл .mrs, как публикуется и скачивается mihomo,
// целиком обёрнут в zstd-фрейм — магическое число Zstandard (RFC 8878 §3.1.1)
// 0x28 0xB5 0x2F 0xFD стоит первым в файле НА ДИСКЕ. "MRS"-заголовок,
// раскладка которого описана ниже, — это первые байты содержимого ПОСЛЕ
// zstd-декомпрессии, а не самого файла. Go stdlib не содержит zstd-декодера
// (только gzip/zlib/flate/bzip2/lzw), а для этой задачи запрещены новые
// зависимости в go.mod — из-за этого ValidateMRSHeader не может заглянуть
// внутрь настоящего zstd-сжатого .mrs глубже магического числа контейнера.
// См. подробное обоснование в док-комментарии ValidateMRSHeader.
//
// Раскладка заголовка ПОСЛЕ zstd-декомпрессии (offset:размер в байтах):
//
//	0:3   magic "MRS" (0x4D 0x52 0x53)
//	3:1   version (во всех образцах = 1)
//	4:1   behavior (0=domain, 1=ipcidr)
//	5:7   зарезервировано/паддинг (во всех образцах — нули)
//	12:8  count, uint64 little-endian (число правил в наборе)
//	20:…  succinct-trie payload — не документирован и не разбирается (skip)
package geo

import (
	"encoding/binary"
	"fmt"
	"io"
)

const (
	// MaxMRSSize — потолок размера .mrs файла (DoS-guard). Реальные .mrs
	// (zstd-сжатые) невелики: от сотен байт (discord.mrs, декомпрессировано —
	// 652 байта) до сотен KB/единиц MB для крупных geoip-категорий (ru.mrs:
	// 118KB сжато / 628KB разжато). 128MB — очень щедрый запас, отсекающий
	// OOM от malicious/повреждённого источника на 128MB-роутере.
	MaxMRSSize int64 = 128 << 20

	mrsMagic = "MRS"

	// mrsHeaderSize — число байт, которые ValidateMRSHeader читает и
	// проверяет структурно в "сыром" (нес-жатом) режиме: magic(3)+version(1)+
	// behavior(1)+reserved(7)+count(8) = 20. Раскладка подтверждена
	// эмпирически на трёх реальных образцах (см. комментарий пакета).
	mrsHeaderSize = 20

	mrsBehaviorDomain byte = 0
	mrsBehaviorIPCIDR byte = 1

	// maxSaneMRSCount — верхняя граница "разумности" count. Крупнейший
	// наблюдавшийся реальный набор (geoip/ru.mrs) — 47457 записей; полный
	// geoip.dat мира на порядок-два больше. 50 млн — щедрый запас: явный
	// мусор (случайные байты, интерпретированные как count) почти всегда даёт
	// значение на порядки больше.
	maxSaneMRSCount = 50_000_000
)

// zstdMagic — магическое число фрейма Zstandard (RFC 8878 §3.1.1): байты на
// диске 0x28 0xB5 0x2F 0xFD (little-endian представление 0xFD2FB528).
var zstdMagic = [4]byte{0x28, 0xB5, 0x2F, 0xFD}

// ValidateMRSHeader проверяет .mrs (mihomo rule-set) заголовок: magic,
// version, behavior и "разумность" count. Только диагностика/pre-flight —
// mihomo сам качает rule-providers, загрузчика в этом пакете нет (см.
// комментарий пакета).
//
// Два режима работы в зависимости от первых 4 байт потока:
//
//  1. Zstd-фрейм (реальный случай для файлов, скачанных mihomo — см.
//     комментарий пакета): декомпрессия невозможна без стороннего
//     zstd-декодера (запрещён новыми зависимостями для этой задачи).
//     ValidateMRSHeader ограничивается распознаванием контейнера и
//     size-guard'ом (maxSize) — глубокая проверка magic/version/behavior/
//     count внутри пропускается. Осознанный компромисс: важнее не отвергать
//     легитимные файлы (которые на практике ВСЕГДА так упакованы), чем
//     полноценно валидировать содержимое без нужной библиотеки.
//  2. Иначе — поток трактуется как уже раскомпрессированный (или
//     синтетический/тестовый) заголовок и проверяется полностью: magic=="MRS",
//     version==1, behavior∈{0,1}, count в разумных пределах.
//
// В обоих режимах общий размер потока не должен превышать maxSize.
// maxSize<=0 использует MaxMRSSize по умолчанию.
func ValidateMRSHeader(r io.Reader, maxSize int64) error {
	if maxSize <= 0 {
		maxSize = MaxMRSSize
	}
	limited := io.LimitReader(r, maxSize+1)

	head := make([]byte, 4)
	n, err := io.ReadFull(limited, head)
	if err != nil {
		return fmt.Errorf("geo: .mrs слишком короткий (%d байт): %w", n, err)
	}

	if [4]byte(head) == zstdMagic {
		return drainMRSWithinLimit(limited, maxSize, int64(len(head)))
	}

	if string(head[:3]) != mrsMagic {
		return fmt.Errorf("geo: неверный magic .mrs: %q (ожидался %q)", head[:3], mrsMagic)
	}
	version := head[3]
	if version != 1 {
		return fmt.Errorf("geo: неизвестная версия .mrs: %d (ожидалась 1)", version)
	}

	rest := make([]byte, mrsHeaderSize-4) // behavior(1)+reserved(7)+count(8) = 16
	if _, err := io.ReadFull(limited, rest); err != nil {
		return fmt.Errorf("geo: .mrs заголовок обрезан: %w", err)
	}
	behavior := rest[0]
	if behavior != mrsBehaviorDomain && behavior != mrsBehaviorIPCIDR {
		return fmt.Errorf("geo: неизвестный behavior .mrs: %d (ожидались 0=domain, 1=ipcidr)", behavior)
	}
	count := binary.LittleEndian.Uint64(rest[8:16])
	if count > maxSaneMRSCount {
		return fmt.Errorf("geo: неразумное количество записей .mrs: %d", count)
	}

	return drainMRSWithinLimit(limited, maxSize, mrsHeaderSize)
}

// drainMRSWithinLimit дочитывает остаток потока (succinct-trie payload — не
// разбирается, только считается) и возвращает ошибку, если суммарный размер
// потока превышает maxSize. limited уже ограничен maxSize+1 суммарно на все
// чтения (включая alreadyRead байт, прочитанных до вызова).
func drainMRSWithinLimit(limited io.Reader, maxSize, alreadyRead int64) error {
	n, err := io.Copy(io.Discard, limited)
	if err != nil {
		return fmt.Errorf("geo: чтение .mrs: %w", err)
	}
	if alreadyRead+n > maxSize {
		return fmt.Errorf("geo: .mrs превышает лимит %d байт", maxSize)
	}
	return nil
}
