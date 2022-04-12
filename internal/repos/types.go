package repos

import (
	"context"
	"encoding/hex"
	"hash/fnv"
	"strings"

	"github.com/sourcegraph/sourcegraph/internal/database"
	"github.com/sourcegraph/sourcegraph/internal/extsvc"
	"github.com/sourcegraph/sourcegraph/internal/ratelimit"
	"github.com/sourcegraph/sourcegraph/internal/types"
	"github.com/sourcegraph/sourcegraph/lib/errors"
)

type externalServiceLister interface {
	List(context.Context, database.ExternalServicesListOptions) ([]*types.ExternalService, error)
}

// RateLimitSyncer syncs rate limits based on external service configuration
type RateLimitSyncer struct {
	registry      *ratelimit.Registry
	serviceLister externalServiceLister
	// How many services to fetch in each DB call
	limit int64
}

// NewRateLimitSyncer returns a new syncer
func NewRateLimitSyncer(registry *ratelimit.Registry, serviceLister externalServiceLister) *RateLimitSyncer {
	r := &RateLimitSyncer{
		registry:      registry,
		serviceLister: serviceLister,
		limit:         500,
	}
	return r
}

// SyncRateLimiters syncs rate limiters for external services, the sync will
// happen for all external services if no IDs are given.
func (r *RateLimitSyncer) SyncRateLimiters(ctx context.Context, ids ...int64) error {
	cursor := database.LimitOffset{
		Limit: int(r.limit),
	}
	for {
		services, err := r.serviceLister.List(ctx,
			database.ExternalServicesListOptions{
				IDs:         ids,
				LimitOffset: &cursor,
			},
		)
		if err != nil {
			return errors.Wrap(err, "listing external services")
		}

		if len(services) == 0 {
			break
		}
		cursor.Offset += len(services)

		for _, svc := range services {
			limit, err := extsvc.ExtractRateLimit(svc.Config, svc.Kind)
			if err != nil {
				if errors.HasType(err, extsvc.ErrRateLimitUnsupported{}) {
					continue
				}
				return errors.Wrap(err, "getting rate limit configuration")
			}

			l := r.registry.Get(svc.URN())
			l.SetLimit(limit)
		}

		if len(services) < int(r.limit) {
			break
		}
	}
	return nil
}

type ScopeCache interface {
	Get(string) ([]byte, bool)
	Set(string, []byte)
}

// GrantedScopes returns a slice of scopes granted by the service based on the token
// provided in the config. It makes a request to the code host but responses are cached
// in Redis based on the token.
//
// Currently only GitHub and GitLab external services with user or org namespace are supported,
// other code hosts will simply return an empty slice
func GrantedScopes(ctx context.Context, cache ScopeCache, db database.DB, svc *types.ExternalService) ([]string, error) {
	externalServicesStore := db.ExternalServices()
	if svc.IsSiteOwned() || (svc.Kind != extsvc.KindGitHub && svc.Kind != extsvc.KindGitLab) {
		return nil, nil
	}
	src, err := NewSource(db, svc, nil)
	if err != nil {
		return nil, errors.Wrap(err, "creating source")
	}
	switch v := src.(type) {
	case *GithubSource:
		// Cached path
		token := v.config.Token
		if token == "" {
			return nil, errors.New("missing token")
		}
		key, err := hashToken(token)
		if err != nil {
			return nil, err
		}
		if result, ok := cache.Get(key); ok && len(result) > 0 {
			return strings.Split(string(result), ","), nil
		}

		// Slow path
		src, err := NewGithubSource(externalServicesStore, svc, nil)
		if err != nil {
			return nil, errors.Wrap(err, "creating source")
		}
		scopes, err := src.v3Client.GetAuthenticatedOAuthScopes(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "getting scopes")
		}
		cache.Set(key, []byte(strings.Join(scopes, ",")))
		return scopes, nil

	case *GitLabSource:
		// Cached path
		token := v.config.Token
		if v.config.TokenType != "oauth" {
			return nil, errors.New("not an oauth token")
		}
		if token == "" {
			return nil, errors.New("missing token")
		}
		key, err := hashToken(token)
		if err != nil {
			return nil, err
		}
		if result, ok := cache.Get(key); ok && len(result) > 0 {
			return strings.Split(string(result), ","), nil
		}

		// Slow path
		src, err := NewGitLabSource(svc, nil)
		if err != nil {
			return nil, errors.Wrap(err, "creating source")
		}
		scopes, err := src.client.GetAuthenticatedUserOAuthScopes(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "getting scopes")
		}
		cache.Set(key, []byte(strings.Join(scopes, ",")))
		return scopes, nil
	default:
		return nil, errors.Errorf("unsupported config type: %T", v)
	}
}

func hashToken(token string) (string, error) {
	h := fnv.New32()
	_, err := h.Write([]byte(token))
	if err != nil {
		return "", errors.Wrap(err, "hashing token")
	}
	b := h.Sum(nil)
	return hex.EncodeToString(b), nil
}
