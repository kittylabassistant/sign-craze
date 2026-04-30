package dpi

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

// makeTarGZ создаёт tarball в памяти с одним файлом по заданному пути.
func makeTarGZ(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	hdr := &tar.Header{
		Name:     name,
		Mode:     0o755,
		Size:     int64(len(content)),
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("tar WriteHeader: %v", err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatalf("tar Write: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar Close: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("gzip Close: %v", err)
	}
	return buf.Bytes()
}

func TestInstall_УстанавливаетБинарь(t *testing.T) {
	binContent := []byte("#!/bin/sh\necho nfqws2")
	tarData := makeTarGZ(t, "nfqws2-v1/nfqws2", binContent)

	dir := t.TempDir()
	tarPath := filepath.Join(dir, "nfqws2.tar.gz")
	if err := os.WriteFile(tarPath, tarData, 0o644); err != nil {
		t.Fatal(err)
	}

	binDst := filepath.Join(dir, "nfqws2")
	if err := Install(tarPath, binDst); err != nil {
		t.Fatalf("Install: %v", err)
	}

	got, err := os.ReadFile(binDst)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(got, binContent) {
		t.Errorf("содержимое бинаря не совпадает")
	}

	info, err := os.Stat(binDst)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0o111 == 0 {
		t.Error("бинарь должен быть исполняемым")
	}
}

func TestInstall_ОшибкаЕслиБинарьОтсутствуетВАрхиве(t *testing.T) {
	// Архив с файлом другого имени
	tarData := makeTarGZ(t, "other-binary", []byte("data"))

	dir := t.TempDir()
	tarPath := filepath.Join(dir, "bad.tar.gz")
	if err := os.WriteFile(tarPath, tarData, 0o644); err != nil {
		t.Fatal(err)
	}

	err := Install(tarPath, filepath.Join(dir, "nfqws2"))
	if err == nil {
		t.Error("ожидалась ошибка при отсутствии 'nfqws2' в архиве")
	}
}

func TestExtractBinary_НаходитВложенныйФайл(t *testing.T) {
	content := []byte("binary-content")
	tarData := makeTarGZ(t, "subdir/nfqws2", content)

	dir := t.TempDir()
	tarPath := filepath.Join(dir, "test.tar.gz")
	dstPath := filepath.Join(dir, "nfqws2")
	if err := os.WriteFile(tarPath, tarData, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := extractBinaryToFile(tarPath, dstPath, 0o755); err != nil {
		t.Fatalf("extractBinaryToFile: %v", err)
	}
	got, _ := os.ReadFile(dstPath)
	if !bytes.Equal(got, content) {
		t.Errorf("содержимое не совпадает")
	}
}
