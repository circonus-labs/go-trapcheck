package trapcheck

import (
	"context"
	"fmt"
	"io"
	"log"
	"reflect"
	"testing"

	"github.com/circonus-labs/go-apiclient"
)

func TestTrapCheck_UpdateCheckTags(t *testing.T) {
	tc := &TrapCheck{}
	tc.Log = &LogWrapper{
		Log:   log.New(io.Discard, "", log.LstdFlags),
		Debug: false,
	}

	tests := []struct {
		client  API
		bundle  *apiclient.CheckBundle
		want    *apiclient.CheckBundle
		name    string
		newTags []string
		wantErr bool
	}{
		{
			name: "new tag",
			bundle: &apiclient.CheckBundle{
				Tags: []string{"foo"},
			},
			newTags: []string{"bar"},
			want: &apiclient.CheckBundle{
				Tags: []string{"foo", "bar"},
			},
			wantErr: false,
			client: &APIMock{
				UpdateCheckBundleFunc: func(cfg *apiclient.CheckBundle) (*apiclient.CheckBundle, error) {
					return cfg, nil
				},
			},
		},
		{
			name: "new tag, ignore blank",
			bundle: &apiclient.CheckBundle{
				Tags: []string{"foo"},
			},
			newTags: []string{"bar", ""},
			want: &apiclient.CheckBundle{
				Tags: []string{"foo", "bar"},
			},
			wantErr: false,
			client: &APIMock{
				UpdateCheckBundleFunc: func(cfg *apiclient.CheckBundle) (*apiclient.CheckBundle, error) {
					return cfg, nil
				},
			},
		},
		{
			name: "modify tag",
			bundle: &apiclient.CheckBundle{
				Tags: []string{"foo:bar"},
			},
			newTags: []string{"foo:baz"},
			want: &apiclient.CheckBundle{
				Tags: []string{"foo:baz"},
			},
			wantErr: false,
			client: &APIMock{
				UpdateCheckBundleFunc: func(cfg *apiclient.CheckBundle) (*apiclient.CheckBundle, error) {
					return cfg, nil
				},
			},
		},
		{
			name: "no change",
			bundle: &apiclient.CheckBundle{
				Tags: []string{"foo"},
			},
			newTags: []string{"foo"},
			want:    nil,
			wantErr: false,
			client: &APIMock{
				UpdateCheckBundleFunc: func(cfg *apiclient.CheckBundle) (*apiclient.CheckBundle, error) {
					return cfg, nil
				},
			},
		},
		{
			name:    "invalid (nil check bundle)",
			bundle:  nil,
			wantErr: true,
		},
		{
			name:    "no tags",
			bundle:  &apiclient.CheckBundle{},
			want:    nil,
			wantErr: false,
		},
		{
			name:    "api error",
			bundle:  &apiclient.CheckBundle{Tags: []string{"foo"}},
			newTags: []string{"bar"},
			want:    nil,
			wantErr: true,
			client: &APIMock{
				UpdateCheckBundleFunc: func(cfg *apiclient.CheckBundle) (*apiclient.CheckBundle, error) {
					return nil, fmt.Errorf("api error 500")
				},
			},
		},
	}
	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			tc.client = tt.client
			tc.checkBundle = tt.bundle

			got, err := tc.UpdateCheckTags(context.Background(), tt.newTags)
			if (err != nil) != tt.wantErr {
				t.Errorf("TrapCheck.UpdateCheckTags() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("TrapCheck.UpdateCheckTags() = %v, want %v", got, tt.want)
			}
		})
	}
}
