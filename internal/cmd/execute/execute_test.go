package execute

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/internal/bundler/archive"
	"code-intelligence.com/cifuzz/pkg/runner/jazzer"
	"code-intelligence.com/cifuzz/pkg/runner/libfuzzer"
)

func Test_getFuzzer(t *testing.T) {
	sampleMetadata := &archive.Metadata{
		Fuzzers: []*archive.Fuzzer{
			{
				Name: "a-fuzzer",
			},
		},
	}
	fuzzer, err := findFuzzer("a-fuzzer", sampleMetadata)
	require.NoError(t, err)
	require.Equal(t, "a-fuzzer", fuzzer.Name)

	fuzzer, err = findFuzzer("b-fuzzer", sampleMetadata)
	require.EqualErrorf(t, err, "fuzzer 'b-fuzzer' not found in a bundle metadata file", "error message mismatch")
}

func Test_getFuzzerName(t *testing.T) {
	type args struct {
		fuzzer *archive.Fuzzer
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "use fuzzer name",
			args: args{
				fuzzer: &archive.Fuzzer{
					Name:   "fuzzer-name",
					Target: "fuzzer-target",
				},
			},
			want: "fuzzer-name",
		},
		{
			name: "use fuzzer target",
			args: args{
				fuzzer: &archive.Fuzzer{
					Name:   "",
					Target: "fuzzer-target",
				},
			},
			want: "fuzzer-target",
		},
		{
			name: "use fuzzer target",
			args: args{
				fuzzer: &archive.Fuzzer{
					Target: "fuzzer-target",
				},
			},
			want: "fuzzer-target",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getFuzzerName(tt.args.fuzzer); got != tt.want {
				t.Errorf("getFuzzerName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_findFuzzer(t *testing.T) {
	type args struct {
		nameToFind     string
		bundleMetadata *archive.Metadata
	}
	tests := []struct {
		name    string
		args    args
		want    *archive.Fuzzer
		wantErr bool
	}{
		{
			name: "find fuzzer by name",
			args: args{
				nameToFind: "a-fuzzer",
				bundleMetadata: &archive.Metadata{
					Fuzzers: []*archive.Fuzzer{
						{
							Name: "0-fuzzer",
						},
						{
							Name: "a-fuzzer",
						},
					},
				},
			},
			want: &archive.Fuzzer{
				Name: "a-fuzzer",
			},
			wantErr: false,
		},
		{
			name: "find fuzzer by target",
			args: args{
				nameToFind: "a-fuzzer",
				bundleMetadata: &archive.Metadata{
					Fuzzers: []*archive.Fuzzer{
						{
							Name: "0-fuzzer",
						},
						{
							Target: "a-fuzzer",
						},
					},
				},
			},
			want: &archive.Fuzzer{
				Target: "a-fuzzer",
			},
			wantErr: false,
		},
		{
			name: "error out if fuzzer not found",
			args: args{
				nameToFind: "a-fuzzer",
				bundleMetadata: &archive.Metadata{
					Fuzzers: []*archive.Fuzzer{
						{
							Name: "0-fuzzer",
						},
					},
				},
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := findFuzzer(tt.args.nameToFind, tt.args.bundleMetadata)
			if (err != nil) != tt.wantErr {
				t.Errorf("findFuzzer() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("findFuzzer() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_buildRunnerJazzerRunner(t *testing.T) {
	runner, err := buildRunner(&archive.Fuzzer{
		Name:   "a-fuzzer",
		Engine: "JAVA_LIBFUZZER",
	})
	require.NoError(t, err)
	v, ok := runner.(*jazzer.Runner)
	require.Equal(t, true, ok)
	require.Equal(t, "a-fuzzer", v.RunnerOptions.TargetClass)
}

func Test_buildRunnerLibfuzzerRunner(t *testing.T) {
	runner, err := buildRunner(&archive.Fuzzer{
		Target: "b-fuzzer",
		Path:   "fuzzTarget",
		Engine: "LIBFUZZER",
	})
	require.NoError(t, err)
	v, ok := runner.(*libfuzzer.Runner)
	require.Equal(t, true, ok)
	require.Equal(t, "fuzzTarget", v.RunnerOptions.FuzzTarget)
}
