package main

import (
	"errors"
	"fmt"
	"time"
)

type Stats struct {
	Repos, Contributed, Stars, Commits, Followers int
	LocAdded, LocDeleted                          int64
	Joined                                        time.Time
}

func (s Stats) Loc() int64 { return s.LocAdded - s.LocDeleted }

const userQuery = `
query($login: String!, $cursor: String) {
  user(login: $login) {
    id
    createdAt
    followers { totalCount }
    repositoriesContributedTo(first: 1, contributionTypes: [COMMIT, PULL_REQUEST, REPOSITORY]) { totalCount }
    repositories(first: 100, after: $cursor, ownerAffiliations: OWNER) {
      totalCount
      pageInfo { hasNextPage endCursor }
      nodes { name stargazerCount isFork }
    }
  }
}`

const locQuery = `
query($owner: String!, $name: String!, $id: ID!, $cursor: String) {
  repository(owner: $owner, name: $name) {
    defaultBranchRef { target { ... on Commit {
      history(first: 100, author: { id: $id }, after: $cursor) {
        pageInfo { hasNextPage endCursor }
        nodes { additions deletions }
      }
    } } }
  }
}`

type count struct{ TotalCount int }

type pageInfo struct {
	HasNextPage bool
	EndCursor   *string
}

type repo struct {
	Name           string
	StargazerCount int
	IsFork         bool
}

type user struct {
	ID                        string
	CreatedAt                 time.Time
	Followers                 count
	RepositoriesContributedTo count
	Repositories              struct {
		TotalCount int
		PageInfo   pageInfo
		Nodes      []repo
	}
}

func fetchStats(gh *gitHubClient, login string) (Stats, error) {
	var u *user
	var repos []repo
	var cursor *string
	for {
		var data struct{ User *user }
		if err := gh.graphql(userQuery, map[string]any{"login": login, "cursor": cursor}, &data); err != nil {
			return Stats{}, err
		}
		if data.User == nil {
			return Stats{}, fmt.Errorf("user %q not found", login)
		}
		u = data.User
		repos = append(repos, u.Repositories.Nodes...)
		cursor = u.Repositories.PageInfo.EndCursor
		if !u.Repositories.PageInfo.HasNextPage {
			break
		}
	}

	s := Stats{
		Repos:       u.Repositories.TotalCount,
		Contributed: u.RepositoriesContributedTo.TotalCount,
		Followers:   u.Followers.TotalCount,
		Joined:      u.CreatedAt.UTC(),
	}
	for _, r := range repos {
		s.Stars += r.StargazerCount
		if r.IsFork {
			continue
		}
		added, deleted, err := linesOfCode(gh, login, r.Name, u.ID)
		if err != nil {
			return Stats{}, err
		}
		s.LocAdded += added
		s.LocDeleted += deleted
	}
	var err error
	s.Commits, err = commits(gh, login, s.Joined.Year())
	return s, err
}

// commits sums commit contributions per year since joining. Private commits count only if the token can see them.
func commits(gh *gitHubClient, login string, joinedYear int) (int, error) {
	years := ""
	for y := joinedYear; y <= time.Now().UTC().Year(); y++ {
		years += fmt.Sprintf("y%d: contributionsCollection(from: \"%d-01-01T00:00:00Z\", to: \"%d-01-01T00:00:00Z\") { totalCommitContributions }\n", y, y, y+1)
	}
	var data struct {
		User map[string]struct{ TotalCommitContributions int }
	}
	query := "query($login: String!) { user(login: $login) { " + years + " } }"
	if err := gh.graphql(query, map[string]any{"login": login}, &data); err != nil {
		return 0, err
	}
	if data.User == nil {
		return 0, errors.New("commits: user missing from response")
	}
	total := 0
	for _, y := range data.User {
		total += y.TotalCommitContributions
	}
	return total, nil
}

// linesOfCode sums additions/deletions of the user's own commits on a repo's default branch.
func linesOfCode(gh *gitHubClient, owner, name, userID string) (added, deleted int64, err error) {
	var cursor *string
	for {
		var data struct {
			Repository *struct {
				DefaultBranchRef *struct {
					Target struct {
						History struct {
							PageInfo pageInfo
							Nodes    []struct{ Additions, Deletions int64 }
						}
					}
				}
			}
		}
		vars := map[string]any{"owner": owner, "name": name, "id": userID, "cursor": cursor}
		if err := gh.graphql(locQuery, vars, &data); err != nil {
			return 0, 0, err
		}
		if data.Repository == nil {
			return 0, 0, fmt.Errorf("repository %s/%s missing from response", owner, name)
		}
		if data.Repository.DefaultBranchRef == nil {
			return added, deleted, nil // empty repo
		}
		history := data.Repository.DefaultBranchRef.Target.History
		for _, n := range history.Nodes {
			added += n.Additions
			deleted += n.Deletions
		}
		if !history.PageInfo.HasNextPage {
			return added, deleted, nil
		}
		cursor = history.PageInfo.EndCursor
	}
}
