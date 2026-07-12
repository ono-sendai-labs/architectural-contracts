package csvfile

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRead(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.csv")
	content := "name,age\nJohn,30\n"
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	malformedFilePath := filepath.Join(tmpDir, "malformed.csv")
	malformedContent := "name,age,extra\nJohn\n"
	if err := os.WriteFile(malformedFilePath, []byte(malformedContent), 0644); err != nil {
		t.Fatalf("failed to write malformed temp file: %v", err)
	}

	tests := []struct {
		name    string
		path    string
		want    [][]string
		wantErr bool
	}{
		{
			name: "successful file read",
			path: filePath,
			want: [][]string{
				{"name", "age"},
				{"John", "30"},
			},
			wantErr: false,
		},
		{
			name:    "nonexistent file error",
			path:    filepath.Join(tmpDir, "nonexistent.csv"),
			want:    nil,
			wantErr: true,
		},
		{
			name:    "malformed CSV error",
			path:    malformedFilePath,
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Read(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("Read() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Read() = %v, want %v", got, tt.want)
			}
		})
	}
}
