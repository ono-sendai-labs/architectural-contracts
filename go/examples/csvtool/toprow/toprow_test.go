package toprow

import (
	"reflect"
	"testing"
)

func TestPick(t *testing.T) {
	tests := []struct {
		name string
		rows [][]string
		col  int
		n    int
		want [][]string
	}{
		{
			name: "basic sort and select descending",
			rows: [][]string{
				{"Alice", "90"},
				{"Bob", "95"},
				{"Charlie", "85"},
			},
			col: 1,
			n:   2,
			want: [][]string{
				{"Bob", "95"},
				{"Alice", "90"},
			},
		},
		{
			name: "selection limit greater than length",
			rows: [][]string{
				{"Alice", "90"},
				{"Bob", "95"},
			},
			col: 1,
			n:   5,
			want: [][]string{
				{"Bob", "95"},
				{"Alice", "90"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Pick(tt.rows, tt.col, tt.n)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Pick() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTopN(t *testing.T) {
	tests := []struct {
		name    string
		csvText string
		col     int
		n       int
		want    [][]string
		wantErr bool
	}{
		{
			name: "valid CSV input",
			csvText: `Alice,90
Bob,95
Charlie,85
`,
			col: 1,
			n:   2,
			want: [][]string{
				{"Bob", "95"},
				{"Alice", "90"},
			},
			wantErr: false,
		},
		{
			name:    "malformed CSV",
			csvText: "Alice,90,extra\nBob",
			col:     1,
			n:       1,
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := TopN(tt.csvText, tt.col, tt.n)
			if (err != nil) != tt.wantErr {
				t.Errorf("TopN() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("TopN() = %v, want %v", got, tt.want)
			}
		})
	}
}
