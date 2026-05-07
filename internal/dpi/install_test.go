package dpi

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

// makeIPK создаёт .ipk пакет в памяти: outer tar.gz → data.tar.gz → бинарь по binPath.
func makeIPK(t *testing.T, binPath string, content []byte) []byte {
	t.Helper()

	// Внутренний data.tar.gz с бинарём.
	dataTarGZ := makeTarGZ(t, binPath, content)

	// Внешний tar.gz, содержащий data.tar.gz.
	var outer bytes.Buffer
	ogw := gzip.NewWriter(&outer)
	otw := tar.NewWriter(ogw)

	hdr := &tar.Header{
		Name:     "./data.tar.gz",
		Mode:     0o644,
		Size:     int64(len(dataTarGZ)),
		Typeflag: tar.TypeReg,
	}
	if err := otw.WriteHeader(hdr); err != nil {
		t.Fatalf("outer tar WriteHeader: %v", err)
	}
	if _, err := otw.Write(dataTarGZ); err != nil {
		t.Fatalf("outer tar Write: %v", err)
	}
	if err := otw.Close(); err != nil {
		t.Fatalf("outer tar Close: %v", err)
	}
	if err := ogw.Close(); err != nil {
		t.Fatalf("outer gzip Close: %v", err)
	}
	return outer.Bytes()
}

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

func TestInstall_IPK_УстанавливаетБинарь(t *testing.T) {
	binContent := []byte("#!/bin/sh\necho nfqws2-ipk")
	ipkData := makeIPK(t, "./usr/sbin/nfqws2", binContent)

	dir := t.TempDir()
	ipkPath := filepath.Join(dir, "nfqws2-keenetic_1.1.5_mipsel-3.4.ipk")
	if err := os.WriteFile(ipkPath, ipkData, 0o644); err != nil {
		t.Fatal(err)
	}

	binDst := filepath.Join(dir, "nfqws2")
	if err := Install(ipkPath, binDst); err != nil {
		t.Fatalf("Install (.ipk): %v", err)
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

// makeIPKWithBlobs создаёт .ipk с бинарём nfqws2 + двумя blob-файлами.
// Используется в TestInstallBlobs.
func makeIPKWithBlobs(t *testing.T) []byte {
	t.Helper()
	// Внутренний data.tar.gz с бинарём + blob'ами.
	var inner bytes.Buffer
	gw := gzip.NewWriter(&inner)
	tw := tar.NewWriter(gw)

	files := map[string][]byte{
		"./usr/sbin/nfqws2":                      []byte("#!/bin/sh\necho fake-nfqws2"),
		"./etc/nfqws2/blobs/quic_initial.bin":    []byte("quic-blob-payload"),
		"./etc/nfqws2/blobs/tls_clienthello.bin": []byte("tls-blob-payload"),
		"./etc/nfqws2/nfqws2.conf":               []byte("# upstream conf"),
	}
	for name, content := range files {
		hdr := &tar.Header{
			Name:     name,
			Mode:     0o644,
			Size:     int64(len(content)),
			Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("inner tar WriteHeader: %v", err)
		}
		if _, err := tw.Write(content); err != nil {
			t.Fatalf("inner tar Write: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("inner tar Close: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("inner gzip Close: %v", err)
	}

	// Внешний tar.gz с data.tar.gz.
	var outer bytes.Buffer
	ogw := gzip.NewWriter(&outer)
	otw := tar.NewWriter(ogw)
	hdr := &tar.Header{
		Name:     "./data.tar.gz",
		Mode:     0o644,
		Size:     int64(inner.Len()),
		Typeflag: tar.TypeReg,
	}
	if err := otw.WriteHeader(hdr); err != nil {
		t.Fatalf("outer tar WriteHeader: %v", err)
	}
	if _, err := otw.Write(inner.Bytes()); err != nil {
		t.Fatalf("outer tar Write: %v", err)
	}
	_ = otw.Close()
	_ = ogw.Close()
	return outer.Bytes()
}

func TestInstallBlobs_РаспаковываетBin(t *testing.T) {
	dir := t.TempDir()
	ipkPath := filepath.Join(dir, "nfqws2.ipk")
	if err := os.WriteFile(ipkPath, makeIPKWithBlobs(t), 0o644); err != nil {
		t.Fatal(err)
	}

	blobDir := filepath.Join(dir, "blobs")
	if err := InstallBlobs(ipkPath, blobDir); err != nil {
		t.Fatalf("InstallBlobs: %v", err)
	}

	checks := map[string]string{
		"quic_initial.bin":    "quic-blob-payload",
		"tls_clienthello.bin": "tls-blob-payload",
	}
	for name, want := range checks {
		got, err := os.ReadFile(filepath.Join(blobDir, name))
		if err != nil {
			t.Errorf("blob %s не извлечён: %v", name, err)
			continue
		}
		if string(got) != want {
			t.Errorf("blob %s: got %q, want %q", name, string(got), want)
		}
	}
}

func TestInstallBlobs_TarGzПропускается(t *testing.T) {
	dir := t.TempDir()
	tarPath := filepath.Join(dir, "nfqws2.tar.gz")
	if err := os.WriteFile(tarPath, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	// .tar.gz путь — InstallBlobs возвращает nil без действий.
	if err := InstallBlobs(tarPath, filepath.Join(dir, "blobs")); err != nil {
		t.Errorf("InstallBlobs не должен падать на .tar.gz: %v", err)
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
