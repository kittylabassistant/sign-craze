package geo

// ValidateDAT/DownloadDAT: работа с .dat (v2ray protobuf geosite/geoip) для xray.
//
// Формат .dat — сериализованный protobuf: GeoSiteList (geosite.dat) или
// GeoIPList (geoip.dat) из схемы v2fly (infra/conf/rule/*.proto). На верхнем
// уровне обе схемы сводятся к одному repeated message-полю:
//
//	message GeoSiteList { repeated GeoSite entry = 1; }
//	message GeoIPList   { repeated GeoIP  entry = 1; }
//
// Wire-format — стандартный protobuf encoding
// (https://protobuf.dev/programming-guides/encoding/): tag = (field_number<<3)|wire_type,
// значения varint — LSB-first, 7 бит на байт, бит 0x80 — признак продолжения.
// Для repeated message-полей (wire_type=2, length-delimited) верхний уровень
// .dat — это просто последовательность записей (tag-varint, length-varint,
// payload) без дополнительной внешней обёртки.
//
// Формат подтверждён эмпирически на реальных файлах
// (Loyalsoldier/v2ray-rules-dat, релиз 202607052256, июль 2026): первые байты
// geosite.dat — `0a f7 01 0a 04 45 53 45 54 12 3d 08 03 12 39 65 73 65 74 ...`
// = tag 0x0a (field=1, wiretype=2), varint-длина 0xf7 0x01 = 247, далее
// вложенный GeoSite{country_code="ESET", domain=[{type=Full(3),
// value="eset-prod-ca..."}]}. Первые байты geoip.dat — `0a c4 27 0a 02 41 44
// 12 08 0a 04 05 3e 3c 05 10 20` = tag/длина 5060, GeoIP{country_code="AD",
// cidr=[{ip=05.3e.3c.05, prefix=32}]}. Реализация ниже НЕ копирует и не
// использует код v2fly/xray/mihomo — только сериализация protobuf-wire-format
// как публичная спецификация (clean-room).
import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/kittylabassistant/sign-craze/internal/atomicfs"
	"github.com/kittylabassistant/sign-craze/internal/ghrelease"
	"github.com/kittylabassistant/sign-craze/internal/log"
	"github.com/kittylabassistant/sign-craze/pkg/types"
)

const (
	// MaxDATSize — потолок размера .dat файла (DoS-guard), применяется и к
	// общему размеру потока, и к каждой отдельной заявленной length-delimited
	// длине внутри protobuf. Реальные geosite.dat/geoip.dat от Loyalsoldier —
	// порядка 10-15MB (наблюдалось: geosite.dat ~10.3MB); 64MB — щедрый запас
	// на будущий рост базы, отсекающий OOM от malicious/повреждённого
	// источника на 128MB-роутере (тот же подход, что MaxGeoFileSize в srs.go).
	MaxDATSize int64 = 64 << 20

	// datRepoOwner/datRepoRepo — источник geosite.dat/geoip.dat для xray.
	// Loyalsoldier/v2ray-rules-dat публикует оба файла и контрольные суммы
	// "<file>.sha256sum" в каждом релизе (тег — дата генерации UTC, например
	// "202607052256"); всегда качаем latest (без pinned tag — база geo
	// обновляется ежедневно апстримом).
	datRepoOwner = "Loyalsoldier"
	datRepoRepo  = "v2ray-rules-dat"
)

// DATFileNames — имена файлов, которые DownloadDAT скачивает и проверяет.
// Экспортировано для CLI (вывод "N/len(DATFileNames) файлов").
var DATFileNames = []string{"geosite.dat", "geoip.dat"}

// ValidateDAT структурно проверяет .dat-файл (v2ray GeoSiteList/GeoIPList) —
// без разбора семантики полей, только форму верхнего уровня protobuf
// wire-format: повторяющуюся последовательность (tag-varint, length-varint,
// payload). Формат — публичная спецификация protobuf encoding
// (https://protobuf.dev/programming-guides/encoding/).
//
// Это НЕ полноценный protobuf-парсер: вложенные сообщения (GeoSite/GeoIP) не
// разбираются — их payload только пропускается (skip) по заявленной длине.
// Этого достаточно, чтобы отличить настоящий .dat от битого/усечённого файла
// или случайного мусора (HTML error-страница вместо asset'а, пустой файл,
// оборванная загрузка), не завязываясь на конкретную схему сообщений
// (форвард-совместимо с любыми будущими полями v2fly proto).
//
// Проверяется для каждой top-level записи:
//   - tag — корректный varint (≤10 байт — предел protobuf для 64-бит значения);
//   - wire_type ∈ {0,1,2,5}: 0=varint, 1=64-bit, 2=length-delimited (основной
//     случай для .dat entries), 5=32-bit — содержимое пропускается согласно
//     спецификации; wire_type ∈ {3,4} (start/end group, deprecated в proto3,
//     v2fly GeoSiteList/GeoIPList их не использует) или другое — отвергается;
//   - для wire_type=2 — заявленная длина не должна уводить за пределы maxSize
//     или оставшегося бюджета;
//   - field number НЕ проверяется (форвард-совместимость: допускаются любые
//     top-level поля, а не только entry=1).
//
// Пустой файл (0 entries) — ошибка: пустой .dat бесполезен для xray и почти
// всегда означает битую/усечённую/неверную загрузку, а не легитимный пустой
// список.
//
// maxSize<=0 использует MaxDATSize по умолчанию.
func ValidateDAT(r io.Reader, maxSize int64) error {
	if maxSize <= 0 {
		maxSize = MaxDATSize
	}

	b := &datBudgetReader{br: bufio.NewReaderSize(r, 4096), limit: maxSize + 1}
	entries := 0

	for {
		tag, err := b.readVarint()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break // чистая граница между entries — конец файла, ок
			}
			return fmt.Errorf("geo: .dat повреждён (entry %d): %w", entries, err)
		}

		wireType := tag & 0x7
		switch wireType {
		case 0: // varint
			if _, err := b.readVarint(); err != nil {
				return fmt.Errorf("geo: .dat повреждён (entry %d, varint-поле): %w", entries, err)
			}
		case 1: // 64-bit (fixed64/double)
			if err := b.skip(8); err != nil {
				return fmt.Errorf("geo: .dat повреждён (entry %d, 64-bit поле): %w", entries, err)
			}
		case 2: // length-delimited — основной случай для GeoSite/GeoIP entries
			length, err := b.readVarint()
			if err != nil {
				return fmt.Errorf("geo: .dat повреждён (entry %d, длина): %w", entries, err)
			}
			if length > uint64(maxSize) {
				return fmt.Errorf("geo: .dat entry %d: длина %d превышает лимит %d байт", entries, length, maxSize)
			}
			if err := b.skip(int64(length)); err != nil {
				return fmt.Errorf("geo: .dat повреждён (entry %d, payload): %w", entries, err)
			}
		case 5: // 32-bit (fixed32/float)
			if err := b.skip(4); err != nil {
				return fmt.Errorf("geo: .dat повреждён (entry %d, 32-bit поле): %w", entries, err)
			}
		default: // 3,4 = start/end group (deprecated proto2), либо мусор
			return fmt.Errorf("geo: .dat entry %d: неподдерживаемый wiretype %d", entries, wireType)
		}
		entries++
	}

	if entries == 0 {
		return fmt.Errorf("geo: .dat пустой (0 записей верхнего уровня)")
	}
	return nil
}

// errDATBudgetExceeded — sentinel, отличающий "превышен лимит размера" от
// чистого io.EOF на границе entry. readVarint должен трактовать их по-разному
// (см. её комментарий), поэтому ошибка НЕ должна быть равна/оборачивать io.EOF.
var errDATBudgetExceeded = errors.New("geo: .dat превышает лимит размера")

// datBudgetReader читает protobuf varint/skip поверх bufio.Reader, считая
// суммарно прочитанные байты против limit (maxSize+1 — см. ValidateDAT).
type datBudgetReader struct {
	br    *bufio.Reader
	used  int64
	limit int64
}

// ReadByte возвращает errDATBudgetExceeded, если бюджет уже исчерпан — ДО
// обращения к нижележащему bufio.Reader (иначе можно прочитать байт, который
// формально "не влезает" в лимит).
func (b *datBudgetReader) ReadByte() (byte, error) {
	if b.used >= b.limit {
		return 0, errDATBudgetExceeded
	}
	c, err := b.br.ReadByte()
	if err != nil {
		return 0, err
	}
	b.used++
	return c, nil
}

// readVarint читает protobuf varint (LSB-first, 7 бит/байт, бит 0x80 —
// признак продолжения). io.EOF/errDATBudgetExceeded НА САМОМ ПЕРВОМ байте —
// "чистая" граница, передаётся вызывающему как есть (БЕЗ %w — иначе
// errors.Is(_, io.EOF) в ValidateDAT ложно сработал бы и на реально оборванный
// varint, если бы io.EOF был обёрнут). EOF/ошибка ПОСЛЕ хотя бы одного
// прочитанного байта — оборванный varint, всегда жёсткая ошибка.
func (b *datBudgetReader) readVarint() (uint64, error) {
	var result uint64
	for i := 0; i < 10; i++ { // 10 байт — предел protobuf для 64-бит varint
		c, err := b.ReadByte()
		if err != nil {
			if i == 0 {
				return 0, err
			}
			// %s + err.Error() (не %w) — намеренно: эта ошибка НЕ должна
			// матчиться errors.Is(_, io.EOF) в ValidateDAT (см. комментарий
			// выше readVarint), в отличие от чистого EOF на границе entry.
			return 0, fmt.Errorf("varint оборван после %d байт: %s", i, err.Error())
		}
		result |= uint64(c&0x7f) << uint(7*i)
		if c&0x80 == 0 {
			return result, nil
		}
	}
	return 0, errors.New("varint длиннее 10 байт (переполнение)")
}

// datSkipChunk — размер одного вызова bufio.Reader.Discard в skip. Discard
// принимает int (32 бита на MIPS32); фиксированный чанк 1MB держит cast
// безопасным независимо от значения n и архитектуры сборки.
const datSkipChunk = 1 << 20

// skip пропускает n байт payload через bufio.Reader.Discard — без выделения
// буфера под сам payload (важно для крупных вложенных GeoSite/GeoIP записей
// на 128MB-роутере).
func (b *datBudgetReader) skip(n int64) error {
	if n < 0 {
		return fmt.Errorf("отрицательная длина %d", n)
	}
	if b.used+n > b.limit {
		return fmt.Errorf("длина %d превышает оставшийся бюджет", n)
	}
	for n > 0 {
		chunk := n
		if chunk > datSkipChunk {
			chunk = datSkipChunk
		}
		d, err := b.br.Discard(int(chunk))
		b.used += int64(d)
		n -= int64(d)
		if err != nil {
			return fmt.Errorf("пропуск payload: %w", err)
		}
	}
	return nil
}

// DownloadDAT скачивает geosite.dat и geoip.dat (v2ray protobuf geo-базы) из
// Loyalsoldier/v2ray-rules-dat для ядра xray (Core.GeoFormat()==GeoDAT).
//
// Почему не через ghrelease.FetchOptions.VerifySHA: встроенная verifySHA256 в
// ghrelease ищет asset "<file>.sha256" (без "sum"), а Loyalsoldier публикует
// "<file>.sha256sum" — стандартный вывод утилиты sha256sum, формат
// "<hex-hash>  <filename>\n" (подтверждено на реальном релизе: содержимое
// geosite.dat.sha256sum — 78 байт, "<64 hex>  geosite.dat\n"). Другая
// конвенция именования, той же природы, что уже встречалась для
// XTLS/Xray-core (zsha256) и MetaCubeX/mihomo (sha256sum.txt) — оба
// потребителя используют AllowMissingSHA=true. Для geo-данных этот путь
// запрещён явно: .dat — подключаемый в маршрутизацию контент, пропуск
// integrity-проверки недопустим. Вместо этого ".sha256sum" скачивается как
// обычный asset через тот же ghrelease.Fetch (без опции VerifySHA), содержимое
// парсится и сверяется вручную.
//
// Путь для каждого файла:
//  1. Fetch(<file>) в cacheDir — ETag-кеш ghrelease работает как обычно.
//  2. Fetch(<file>.sha256sum) в cacheDir.
//  3. Сверка SHA256 (та же идея, что verifySHA256 в ghrelease, но с разбором
//     суффикса "sum").
//  4. Если dstDir уже содержит файл с тем же SHA256 — пропуск (идемпотентность,
//     как Update/srs.go).
//  5. ValidateDAT — структурная проверка ДО замены боевого файла (защита от
//     подписанного, но повреждённого/не-dat содержимого).
//  6. Атомарная запись в dstDir через atomicfs — production-копия отделена от
//     cache, поэтому невалидная загрузка не может испортить последний рабочий
//     .dat.
//
// Возвращает число реально обновлённых файлов (0..len(DATFileNames)).
func DownloadDAT(ctx context.Context, cacheDir, dstDir string) (int, error) {
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return 0, fmt.Errorf("geo: создание cache dir %s: %w", cacheDir, err)
	}
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return 0, fmt.Errorf("geo: создание dst dir %s: %w", dstDir, err)
	}

	dl := ghrelease.New()
	updated := 0
	for _, name := range DATFileNames {
		changed, err := downloadOneDAT(ctx, dl, cacheDir, dstDir, name)
		if err != nil {
			return updated, fmt.Errorf("geo: %s: %w", name, err)
		}
		if changed {
			updated++
		}
	}
	return updated, nil
}

func downloadOneDAT(ctx context.Context, dl *ghrelease.Downloader, cacheDir, dstDir, name string) (bool, error) {
	assetRes, err := dl.Fetch(ctx, ghrelease.FetchOptions{
		Owner:      datRepoOwner,
		Repo:       datRepoRepo,
		AssetMatch: exactAssetName(name),
		DstDir:     cacheDir,
	})
	if err != nil {
		return false, fmt.Errorf("скачивание: %w", err)
	}

	sumName := name + ".sha256sum"
	sumRes, err := dl.Fetch(ctx, ghrelease.FetchOptions{
		Owner:      datRepoOwner,
		Repo:       datRepoRepo,
		AssetMatch: exactAssetName(sumName),
		DstDir:     cacheDir,
	})
	if err != nil {
		return false, fmt.Errorf("скачивание %s: %w", sumName, err)
	}

	wantSum, err := readSHA256SumFile(sumRes.Path)
	if err != nil {
		return false, fmt.Errorf("контрольная сумма %s: %w", sumName, err)
	}
	gotSum := localSHA256(assetRes.Path)
	if gotSum == "" {
		return false, fmt.Errorf("не удалось вычислить SHA256 %s", assetRes.Path)
	}
	if !strings.EqualFold(gotSum, wantSum) {
		_ = os.Remove(assetRes.Path)
		return false, fmt.Errorf("SHA256 не совпадает: ожидался %s, получен %s", wantSum, gotSum)
	}

	dstPath := filepath.Join(dstDir, name)
	if localSHA256(dstPath) == gotSum {
		log.L().Debug("geo dat: файл актуален, пропускаем", "name", name)
		return false, nil
	}

	if verr := validateDATFile(assetRes.Path); verr != nil {
		// Не оставляем в cache подписанный, но структурно битый файл — иначе
		// ETag-кеш ghrelease решит, что "ничего не изменилось", и следующий
		// вызов будет молча повторять ту же ошибку без попытки перекачать.
		_ = os.Remove(assetRes.Path)
		return false, fmt.Errorf("структурная проверка: %w", verr)
	}

	src, err := os.Open(assetRes.Path)
	if err != nil {
		return false, fmt.Errorf("открытие для записи: %w", err)
	}
	defer src.Close()
	if err := atomicfs.WriteFileAtomicFromReader(dstPath, src, 0o644); err != nil {
		return false, fmt.Errorf("атомарная запись %s: %w", dstPath, err)
	}

	log.L().Info("geo dat: файл обновлён", "name", name, "sha256", gotSum[:12]+"...")
	return true, nil
}

// validateDATFile открывает path и прогоняет ValidateDAT (MaxDATSize).
func validateDATFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("открытие %s: %w", path, err)
	}
	defer f.Close()
	return ValidateDAT(f, MaxDATSize)
}

// exactAssetName возвращает AssetMatch для точного совпадения имени asset'а.
// ghrelease.MatchByContains здесь не подходит: нужен строгий, наименее хрупкий
// контракт для заранее известных, фиксированных имён файлов (а не
// "содержит подстроку", как для архивов бинарей разных архитектур).
func exactAssetName(name string) func(types.Asset) bool {
	return func(a types.Asset) bool { return a.Name == name }
}

// readSHA256SumFile читает файл в формате утилиты `sha256sum`
// ("<hex>  <filename>\n") и возвращает hex-хэш (первое поле). Формат
// подтверждён на реальном релизе Loyalsoldier/v2ray-rules-dat.
func readSHA256SumFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("чтение %s: %w", path, err)
	}
	fields := strings.Fields(strings.TrimSpace(string(data)))
	if len(fields) == 0 {
		return "", fmt.Errorf("пустой файл контрольной суммы")
	}
	return fields[0], nil
}
