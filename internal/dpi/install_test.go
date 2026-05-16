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

// makeGZBytes сжимает data в gzip и возвращает сжатые байты.
func makeGZBytes(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	if _, err := gw.Write(data); err != nil {
		t.Fatalf("gzip Write: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("gzip Close: %v", err)
	}
	return buf.Bytes()
}

// makeIPKWithAssets создаёт .ipk с blob (.bin) и lua (.lua.gz) файлами.
func makeIPKWithAssets(t *testing.T, blobs map[string][]byte, luas map[string][]byte) []byte {
	t.Helper()

	// Строим data.tar.gz с assets.
	var dataBuf bytes.Buffer
	dgw := gzip.NewWriter(&dataBuf)
	dtw := tar.NewWriter(dgw)

	writeEntry := func(name string, content []byte) {
		hdr := &tar.Header{
			Name:     name,
			Mode:     0o644,
			Size:     int64(len(content)),
			Typeflag: tar.TypeReg,
		}
		if err := dtw.WriteHeader(hdr); err != nil {
			t.Fatalf("tar WriteHeader %s: %v", name, err)
		}
		if _, err := dtw.Write(content); err != nil {
			t.Fatalf("tar Write %s: %v", name, err)
		}
	}

	for name, data := range blobs {
		writeEntry("./etc/nfqws2/blobs/"+name, data)
	}
	for name, data := range luas {
		// Упаковываем lua-содержимое в gzip (формат .lua.gz).
		writeEntry("./etc/nfqws2/lua/"+name+".gz", makeGZBytes(t, data))
	}

	if err := dtw.Close(); err != nil {
		t.Fatalf("data tar Close: %v", err)
	}
	if err := dgw.Close(); err != nil {
		t.Fatalf("data gzip Close: %v", err)
	}

	// Упаковываем data.tar.gz в outer tar.gz.
	dataTarGZ := dataBuf.Bytes()
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
		t.Fatalf("outer WriteHeader: %v", err)
	}
	if _, err := otw.Write(dataTarGZ); err != nil {
		t.Fatalf("outer Write: %v", err)
	}
	if err := otw.Close(); err != nil {
		t.Fatalf("outer tar Close: %v", err)
	}
	if err := ogw.Close(); err != nil {
		t.Fatalf("outer gzip Close: %v", err)
	}
	return outer.Bytes()
}

func TestInstallAssets_ИзвлекаетBlobИLua(t *testing.T) {
	blobContent := []byte("blob-binary-payload")
	luaContent := []byte("-- antidpi lua script\nreturn {}")

	ipkData := makeIPKWithAssets(t,
		map[string][]byte{"fake.bin": blobContent},
		map[string][]byte{"antidpi.lua": luaContent},
	)

	dir := t.TempDir()
	ipkPath := filepath.Join(dir, "nfqws2.ipk")
	if err := os.WriteFile(ipkPath, ipkData, 0o644); err != nil {
		t.Fatal(err)
	}

	blobDir := filepath.Join(dir, "blobs")
	luaDir := filepath.Join(dir, "lua")

	if err := InstallAssets(ipkPath, blobDir, luaDir); err != nil {
		t.Fatalf("InstallAssets: %v", err)
	}

	// Проверяем blob.
	gotBlob, err := os.ReadFile(filepath.Join(blobDir, "fake.bin"))
	if err != nil {
		t.Fatalf("ReadFile blob: %v", err)
	}
	if !bytes.Equal(gotBlob, blobContent) {
		t.Errorf("blob содержимое не совпадает")
	}

	// Проверяем lua (должен быть разархивирован .lua.gz → .lua).
	gotLua, err := os.ReadFile(filepath.Join(luaDir, "antidpi.lua"))
	if err != nil {
		t.Fatalf("ReadFile lua: %v", err)
	}
	if !bytes.Equal(gotLua, luaContent) {
		t.Errorf("lua содержимое не совпадает: got %q, want %q", gotLua, luaContent)
	}

	// Проверяем права доступа.
	info, err := os.Stat(filepath.Join(luaDir, "antidpi.lua"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Errorf("lua права = %o, ожидалось 0644", info.Mode().Perm())
	}
}

func TestInstallAssets_ПропускаетНе_IPK(t *testing.T) {
	dir := t.TempDir()
	tarPath := filepath.Join(dir, "nfqws2.tar.gz")
	tarData := makeTarGZ(t, "nfqws2", []byte("binary"))
	if err := os.WriteFile(tarPath, tarData, 0o644); err != nil {
		t.Fatal(err)
	}

	// Для .tar.gz — no-op, ошибки нет.
	if err := InstallAssets(tarPath, dir, dir); err != nil {
		t.Errorf("InstallAssets для .tar.gz должен быть no-op, получили: %v", err)
	}
}
