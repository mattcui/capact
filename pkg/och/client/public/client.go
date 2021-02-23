package public

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/avast/retry-go"
	gqlpublicapi "projectvoltron.dev/voltron/pkg/och/api/graphql/public"

	"github.com/machinebox/graphql"
	"github.com/pkg/errors"
)

const retryAttempts = 1

// Client used to communicate with the Voltron Public OCH GraphQL APIs
type Client struct {
	client *graphql.Client
}

func NewClient(cli *graphql.Client) *Client {
	return &Client{client: cli}
}

// ListInterfacesMetadata returns only name, prefix and path. Rest fields have zero value.
func (c *Client) ListInterfacesMetadata(ctx context.Context) ([]gqlpublicapi.Interface, error) {
	req := graphql.NewRequest(`query {
		interfaces {
			name
			prefix
			path
		}		
	}`)

	var resp struct {
		Interfaces []gqlpublicapi.Interface `json:"interfaces"`
	}
	err := retry.Do(func() error {
		return c.client.Run(ctx, req, &resp)
	}, retry.Attempts(retryAttempts))
	if err != nil {
		return nil, errors.Wrap(err, "while executing query to fetch OCH Implementation")
	}

	return resp.Interfaces, nil
}

func (c *Client) GetInterfaceRevision(ctx context.Context, ref gqlpublicapi.InterfaceReference) (*gqlpublicapi.InterfaceRevision, error) {
	query, params := c.interfaceQueryForRef(ref)
	req := graphql.NewRequest(fmt.Sprintf(`query($interfacePath: NodePath!, %s) {
		  interface(path: $interfacePath) {
				%s
		  }
		}`, params.Query(), query))

	req.Var("interfacePath", ref.Path)
	params.PopulateVars(req)

	var resp struct {
		Interface struct {
			Revision gqlpublicapi.InterfaceRevision `json:"rev"`
		} `json:"interface"`
	}
	err := retry.Do(func() error {
		return c.client.Run(ctx, req, &resp)
	}, retry.Attempts(retryAttempts))

	if err != nil {
		return nil, errors.Wrap(err, "while executing query to fetch OCH Interface Revision")
	}

	return &resp.Interface.Revision, nil
}

func (c *Client) GetInterfaceLatestRevisionString(ctx context.Context, ref gqlpublicapi.InterfaceReference) (string, error) {
	req := graphql.NewRequest(`query ($interfacePath: NodePath!) {
		interface(path: $interfacePath) {
			latestRevision {
				revision
			}
		}		
	}`)

	req.Var("interfacePath", ref.Path)

	var resp struct {
		Interface struct {
			LatestRevision struct {
				Revision string `json:"revision"`
			} `json:"latestRevision"`
		} `json:"interface"`
	}
	err := retry.Do(func() error {
		return c.client.Run(ctx, req, &resp)
	}, retry.Attempts(retryAttempts))
	if err != nil {
		return "", errors.Wrap(err, "while executing query to fetch Interface latest revision string")
	}

	return resp.Interface.LatestRevision.Revision, nil
}

func (c *Client) GetImplementationRevisionsForInterface(ctx context.Context, ref gqlpublicapi.InterfaceReference, opts ...GetImplementationOption) ([]gqlpublicapi.ImplementationRevision, error) {
	getOpts := &GetImplementationOptions{}
	getOpts.Apply(opts...)

	query, params := c.interfaceQueryForRef(ref)
	req := graphql.NewRequest(fmt.Sprintf(`query($interfacePath: NodePath!, %s) {
		  interface(path: $interfacePath) {
				%s
		  }
		}`, params.Query(), query))

	req.Var("interfacePath", ref.Path)
	params.PopulateVars(req)

	var resp struct {
		Interface struct {
			LatestRevision struct {
				ImplementationRevisions []gqlpublicapi.ImplementationRevision `json:"implementationRevisions"`
			} `json:"rev"`
		} `json:"interface"`
	}
	err := retry.Do(func() error {
		return c.client.Run(ctx, req, &resp)
	}, retry.Attempts(retryAttempts))

	if err != nil {
		return nil, errors.Wrap(err, "while executing query to fetch OCH Implementation")
	}

	result := FilterImplementationRevisions(resp.Interface.LatestRevision.ImplementationRevisions, getOpts)
	if len(result) == 0 {
		return nil, NewImplementationRevisionNotFoundError(ref)
	}

	return result, nil
}

var key = regexp.MustCompile(`\$(\w+):`)

type Args map[string]interface{}

func (a Args) Query() string {
	var out []string
	for k := range a {
		out = append(out, k)
	}
	return strings.Join(out, ",")
}

func (a Args) PopulateVars(req *graphql.Request) {
	for k, v := range a {
		name := key.FindStringSubmatch(k)
		req.Var(name[1], v)
	}
}

func (c *Client) interfaceQueryForRef(ref gqlpublicapi.InterfaceReference) (string, Args) {
	if ref.Revision == "" {
		return c.latestInterfaceRevision()
	}

	return c.specificInterfaceRevision(ref.Revision)
}

func (c *Client) latestInterfaceRevision() (string, Args) {
	latestRevision := fmt.Sprintf(`
			rev: latestRevision {
				%s
			}`, InterfaceRevisionFields)

	return latestRevision, Args{}
}

func (c *Client) specificInterfaceRevision(rev string) (string, Args) {
	specificRevision := fmt.Sprintf(`
			rev: revision(revision: $interfaceRev) {
				%s
			}`, InterfaceRevisionFields)

	return specificRevision, Args{
		"$interfaceRev: Version!": rev,
	}
}
