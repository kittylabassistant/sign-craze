// Package corearchive содержит общий каркас установки бинарей прокси-ядер
// (xray, mihomo) из архивов: потоковый BinaryStream, ELF-magic проверку и
// рамку InstallWithRollback (backup → атомарная запись → опциональная
// валидация конфига → откат при неудаче).
//
// Формат архива (zip у xray, plain gzip у mihomo) и способ поиска бинаря
// внутри него остаются специфичными для каждого ядра — эта логика в
// corearchive не входит, только то, что после открытия архива и до старта
// нового бинаря одинаково для всех.
//
// singbox не использует этот пакет: у него другая архитектура — валидация
// конфига ДО подмены бинаря (см. internal/singbox.PrepareAndValidate), а не
// backup-then-validate-then-rollback.
package corearchive
