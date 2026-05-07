package singbox

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestSingboxReleaseAssetAndDownloadURL(t *testing.T) {
	asset := buildSingboxReleaseAsset("1.10.7", "linux", "amd64")
	if asset != "sing-box-1.10.7-linux-amd64.tar.gz" {
		t.Fatalf("asset = %q", asset)
	}

	url := buildSingboxDownloadURL("1.10.7", "linux", "amd64")
	want := "https://github.com/SagerNet/sing-box/releases/download/v1.10.7/sing-box-1.10.7-linux-amd64.tar.gz"
	if url != want {
		t.Fatalf("download url = %q, want %q", url, want)
	}
}

func TestExtractSingboxBinaryFromTarGz(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "sing-box.tar.gz")
	targetPath := filepath.Join(dir, "sing-box")

	if err := writeTestSingboxArchive(archivePath, "fake-sing-box-binary"); err != nil {
		t.Fatalf("write archive: %v", err)
	}

	if err := extractSingboxBinaryFromTarGz(archivePath, targetPath); err != nil {
		t.Fatalf("extractSingboxBinaryFromTarGz returned error: %v", err)
	}

	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read extracted binary: %v", err)
	}
	if string(data) != "fake-sing-box-binary" {
		t.Fatalf("extracted binary = %q", string(data))
	}

	info, err := os.Stat(targetPath)
	if err != nil {
		t.Fatalf("stat extracted binary: %v", err)
	}
	if info.Mode()&0111 == 0 {
		t.Fatalf("extracted binary is not executable: mode %v", info.Mode())
	}
}

func writeTestSingboxArchive(path, content string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	gz := gzip.NewWriter(file)
	defer gz.Close()

	tw := tar.NewWriter(gz)
	defer tw.Close()

	data := []byte(content)
	if err := tw.WriteHeader(&tar.Header{
		Name: "sing-box-1.10.7-linux-amd64/sing-box",
		Mode: 0755,
		Size: int64(len(data)),
	}); err != nil {
		return err
	}
	_, err = tw.Write(data)
	return err
}
