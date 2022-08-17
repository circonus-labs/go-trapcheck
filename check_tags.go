package trapcheck

import (
	"context"
	"fmt"
	"strings"

	"github.com/circonus-labs/go-apiclient"
)

func (tc *TrapCheck) UpdateCheckTags(ctx context.Context, tags []string) (*apiclient.CheckBundle, error) {
	if tc.checkBundle == nil {
		return nil, fmt.Errorf("invalid state, check bundle is nil")
	}
	if len(tags) == 0 {
		return nil, nil
	}

	update := false
	for _, tag := range tags {
		if tag == "" {
			continue
		}
		found := false
		tagParts := strings.SplitN(tag, ":", 2)
		for j, ctag := range tc.checkBundle.Tags {
			if tag == ctag {
				found = true
				break
			}

			ctagParts := strings.SplitN(ctag, ":", 2)
			if len(tagParts) == len(ctagParts) {
				if tagParts[0] == ctagParts[0] {
					if tagParts[1] != ctagParts[1] {
						tc.Log.Warnf("modifying tag: new: %v old: %v", tagParts, ctagParts)
						tc.checkBundle.Tags[j] = tag
						update = true // but force update since we're modifying a tag
						found = true
						break
					}
				}

			}
		}
		if !found {
			tc.Log.Warnf("adding missing tag: %s curr: %v", tag, tc.checkBundle.Tags)
			tc.checkBundle.Tags = append(tc.checkBundle.Tags, tag)
			update = true
		}
	}

	if update {
		b, err := tc.client.UpdateCheckBundle(tc.checkBundle)
		if err != nil {
			return nil, fmt.Errorf("api updating check bundle tags: %w", err)
		}
		return b, nil
	}

	return nil, nil
}
