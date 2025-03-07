package gitea

import (
	"strings"
	"time"

	"code.gitea.io/sdk/gitea"
	"github.com/cooperspencer/gickup/types"
	"github.com/rs/zerolog/log"
)

func getOrgVisibility(visibility string) gitea.VisibleType {
	switch visibility {
	case "public":
		return gitea.VisibleTypePublic
	case "private":
		return gitea.VisibleTypePrivate
	case "limited":
		return gitea.VisibleTypeLimited
	default:
		return gitea.VisibleTypePrivate
	}
}

func getRepoVisibility(visibility string) bool {
	switch visibility {
	case "public":
		return false
	case "private":
		return true
	default:
		return true
	}
}

// Backup TODO.
func Backup(r types.Repo, d types.GenRepo, dry bool) bool {
	orgvisibilty := getOrgVisibility(d.Visibility.Organizations)
	repovisibility := getRepoVisibility(d.Visibility.Repositories)
	if d.URL == "" {
		d.URL = "https://gitea.com/"
	}

	log.Info().
		Str("stage", "gitea").
		Str("url", d.URL).
		Msgf("mirroring %s to %s", types.Blue(r.Name), d.URL)

	giteaclient, err := gitea.NewClient(d.URL, gitea.SetToken(d.GetToken()))
	if err != nil {
		log.Error().Str("stage", "gitea").Str("url", d.URL).Msg(err.Error())
		return false
	}

	user, _, err := giteaclient.GetMyUserInfo()
	if err != nil {
		log.Error().
			Str("stage", "gitea").
			Str("url", d.URL).
			Msg(err.Error())
		return false
	}

	if d.User == "" && d.CreateOrg {
		d.User = r.Owner
	}

	if d.User != "" {
		user, _, err = giteaclient.GetUserInfo(d.User)
		if err != nil {
			if d.CreateOrg {
				org, _, err := giteaclient.CreateOrg(gitea.CreateOrgOption{
					Name:       d.User,
					Visibility: orgvisibilty,
				})
				if err != nil {
					log.Error().
						Str("stage", "gitea").
						Str("url", d.URL).
						Msg(err.Error())
					return false
				}
				user.ID = org.ID
				user.UserName = org.UserName
			} else {
				log.Error().
					Str("stage", "gitea").
					Str("url", d.URL).
					Msg(err.Error())
				return false
			}
		}

	}

	if dry {
		return true
	}

	repo, _, err := giteaclient.GetRepo(user.UserName, r.Name)
	if err != nil {
		opts := gitea.MigrateRepoOption{
			RepoName:  r.Name,
			RepoOwner: user.UserName,
			Mirror:    true,
			CloneAddr: r.URL,
			AuthToken: r.Token,
			Wiki:      r.Origin.Wiki,
			Private:   repovisibility,
		}

		if r.Token == "" {
			opts = gitea.MigrateRepoOption{
				RepoName:     r.Name,
				RepoOwner:    user.UserName,
				Mirror:       true,
				CloneAddr:    r.URL,
				AuthUsername: r.Origin.User,
				AuthPassword: r.Origin.Password,
				Wiki:         r.Origin.Wiki,
				Private:      repovisibility,
			}
		}

		_, _, err := giteaclient.MigrateRepo(opts)
		if err != nil {
			log.Error().
				Str("stage", "gitea").
				Str("url", d.URL).
				Msg(err.Error())
			return false
		}

		log.Info().
			Str("stage", "gitea").
			Str("url", d.URL).
			Msgf("mirrored %s to %s", types.Blue(r.Name), d.URL)

		return true
	}
	if repo.Mirror {
		log.Info().
			Str("stage", "gitea").
			Str("url", d.URL).
			Msgf("mirror of %s already exists, syncing instead", types.Blue(r.Name))

		_, err := giteaclient.MirrorSync(user.UserName, repo.Name)
		if err != nil {
			log.Error().
				Str("stage", "gitea").
				Str("url", d.URL).
				Msg(err.Error())
			return false
		}

		log.Info().
			Str("stage", "gitea").
			Str("url", d.URL).
			Msgf("successfully synced %s.", types.Blue(r.Name))
	}

	return true
}

// Get TODO.
func Get(conf *types.Conf) ([]types.Repo, bool) {
	ran := false
	repos := []types.Repo{}
	for _, repo := range conf.Source.Gitea {
		err := repo.Filter.ParseDuration()
		if err != nil {
			log.Error().
				Str("stage", "gitea").
				Str("url", repo.URL).
				Msg(err.Error())
		}
		ran = true
		if repo.URL == "" {
			repo.URL = "https://gitea.com"
		}
		if repo.User == "" {
			log.Info().
				Str("stage", "gitea").
				Str("url", repo.URL).
				Msg("grabbing my repositories")
		} else {
			log.Info().
				Str("stage", "gitea").
				Str("url", repo.URL).
				Msgf("grabbing repositories from %s", repo.User)
		}
		opt := gitea.ListReposOptions{}
		opt.PageSize = 50
		opt.Page = 1
		gitearepos := []*gitea.Repository{}

		var client *gitea.Client
		token := repo.GetToken()
		if token != "" {
			client, err = gitea.NewClient(repo.URL, gitea.SetToken(token))
		} else {
			client, err = gitea.NewClient(repo.URL)
		}

		if token != "" && repo.User == "" {
			user, _, err := client.GetMyUserInfo()
			if err != nil {
				log.Error().
					Str("stage", "gitea").
					Str("url", repo.URL).
					Msg(err.Error())
				continue
			}
			repo.User = user.UserName
		}

		if err != nil {
			log.Error().
				Str("stage", "gitea").
				Str("url", repo.URL).
				Msg(err.Error())
			continue
		}

		for {
			repos, _, err := client.ListUserRepos(repo.User, opt)
			if err != nil {
				log.Error().
					Str("stage", "gitea").
					Str("url", repo.URL).
					Msg(err.Error())
				continue
			}
			if len(repos) == 0 {
				break
			}
			gitearepos = append(gitearepos, repos...)
			opt.Page++
		}

		if repo.Starred {
			starredrepos, _, err := client.GetStarredRepos(repo.User)
			if err != nil {
				log.Error().
					Str("stage", "gitea").
					Str("url", repo.URL).
					Msg(err.Error())
			} else {
				gitearepos = append(gitearepos, starredrepos...)
			}
		}

		include := types.GetMap(repo.Include)
		exclude := types.GetMap(repo.Exclude)
		includeorgs := types.GetMap(repo.IncludeOrgs)
		excludeorgs := types.GetMap(repo.ExcludeOrgs)
		for i := range repo.Filter.Languages {
			repo.Filter.Languages[i] = strings.ToLower(repo.Filter.Languages[i])
		}
		languages := types.GetMap(repo.Filter.Languages)

		for _, r := range gitearepos {
			if repo.Filter.ExcludeArchived {
				if r.Archived {
					continue
				}
			}

			if len(repo.Filter.Languages) > 0 {
				langs, _, err := client.GetRepoLanguages(r.Owner.UserName, r.Name)
				if err != nil {
					log.Error().
						Str("stage", "gitea").
						Str("url", repo.URL).
						Msg(err.Error())
					continue
				} else {
					language := ""
					percentage := int64(0)
					for lang, percent := range langs {
						if percent > percentage {
							language = lang
						}
					}
					if !languages[strings.ToLower(language)] {
						continue
					}
				}
			}

			if r.Stars < repo.Filter.Stars {
				continue
			}
			if time.Since(r.Updated) > repo.Filter.LastActivityDuration && repo.Filter.LastActivityDuration != 0 {
				continue
			}
			if include[r.Name] {
				repos = append(repos, types.Repo{
					Name:          r.Name,
					URL:           r.CloneURL,
					SSHURL:        r.SSHURL,
					Token:         token,
					Defaultbranch: r.DefaultBranch,
					Origin:        repo,
					Owner:         r.Owner.UserName,
					Hoster:        types.GetHost(repo.URL),
				})
				if r.HasWiki && repo.Wiki && types.StatRemote(r.CloneURL, r.SSHURL, repo) {
					repos = append(repos, types.Repo{
						Name:          r.Name + ".wiki",
						URL:           types.DotGitRx.ReplaceAllString(r.CloneURL, ".wiki.git"),
						SSHURL:        types.DotGitRx.ReplaceAllString(r.SSHURL, ".wiki.git"),
						Token:         token,
						Defaultbranch: r.DefaultBranch,
						Origin:        repo,
						Owner:         r.Owner.UserName,
						Hoster:        types.GetHost(repo.URL),
					})
				}

				continue
			}

			if exclude[r.Name] {
				continue
			}

			if len(repo.Include) == 0 {
				repos = append(repos, types.Repo{
					Name:          r.Name,
					URL:           r.CloneURL,
					SSHURL:        r.SSHURL,
					Token:         token,
					Defaultbranch: r.DefaultBranch,
					Origin:        repo,
					Owner:         r.Owner.UserName,
					Hoster:        types.GetHost(repo.URL),
				})
				if r.HasWiki && repo.Wiki && types.StatRemote(r.CloneURL, r.SSHURL, repo) {
					repos = append(repos, types.Repo{
						Name:          r.Name + ".wiki",
						URL:           types.DotGitRx.ReplaceAllString(r.CloneURL, ".wiki.git"),
						SSHURL:        types.DotGitRx.ReplaceAllString(r.SSHURL, ".wiki.git"),
						Token:         token,
						Defaultbranch: r.DefaultBranch,
						Origin:        repo,
						Owner:         r.Owner.UserName,
						Hoster:        types.GetHost(repo.URL),
					})
				}
			}
		}
		orgopt := gitea.ListOptions{Page: 1, PageSize: 50}
		orgs := []*gitea.Organization{}
		if token != "" {
			for {
				o, _, err := client.ListUserOrgs(repo.User, gitea.ListOrgsOptions{ListOptions: orgopt})
				if err != nil {
					log.Error().
						Str("stage", "gitea").
						Str("url", repo.URL).
						Msg(err.Error())
				}
				if len(o) == 0 {
					break
				}
				orgs = append(orgs, o...)
				orgopt.Page++
			}
		}
		orgrepos := []*gitea.Repository{}
		for _, org := range orgs {
			orgopt.Page = 1
			if excludeorgs[org.UserName] {
				continue
			}
			for {
				if len(includeorgs) > 0 {
					if includeorgs[org.UserName] {
						o := getOrgRepos(client, org, orgopt, repo)
						if len(o) == 0 {
							break
						}
						orgrepos = append(orgrepos, o...)
					}
				} else {
					o := getOrgRepos(client, org, orgopt, repo)
					if len(o) == 0 {
						break
					}
					orgrepos = append(orgrepos, o...)
				}
				orgopt.Page++
			}
		}
		for _, r := range orgrepos {
			if repo.Filter.ExcludeArchived {
				if r.Archived {
					continue
				}
			}

			if len(repo.Filter.Languages) > 0 {
				langs, _, err := client.GetRepoLanguages(r.Owner.UserName, r.Name)
				if err != nil {
					log.Error().
						Str("stage", "gitea").
						Str("url", repo.URL).
						Msg(err.Error())
					continue
				} else {
					language := ""
					percentage := int64(0)
					for lang, percent := range langs {
						if percent > percentage {
							language = lang
						}
					}
					if !languages[strings.ToLower(language)] {
						continue
					}
				}
			}

			if r.Stars < repo.Filter.Stars {
				continue
			}
			if time.Since(r.Updated) > repo.Filter.LastActivityDuration && repo.Filter.LastActivityDuration != 0 {
				continue
			}
			if include[r.Name] {
				repos = append(repos, types.Repo{
					Name:          r.Name,
					URL:           r.CloneURL,
					SSHURL:        r.SSHURL,
					Token:         token,
					Defaultbranch: r.DefaultBranch,
					Origin:        repo,
					Owner:         r.Owner.UserName,
					Hoster:        types.GetHost(repo.URL),
				})
				if r.HasWiki && repo.Wiki && types.StatRemote(r.CloneURL, r.SSHURL, repo) {
					repos = append(repos, types.Repo{
						Name:          r.Name + ".wiki",
						URL:           types.DotGitRx.ReplaceAllString(r.CloneURL, ".wiki.git"),
						SSHURL:        types.DotGitRx.ReplaceAllString(r.SSHURL, ".wiki.git"),
						Token:         token,
						Defaultbranch: r.DefaultBranch,
						Origin:        repo,
						Owner:         r.Owner.UserName,
						Hoster:        types.GetHost(repo.URL),
					})
				}

				continue
			}

			if exclude[r.Name] {
				continue
			}

			if len(repo.Include) == 0 {
				repos = append(repos, types.Repo{
					Name:          r.Name,
					URL:           r.CloneURL,
					SSHURL:        r.SSHURL,
					Token:         token,
					Defaultbranch: r.DefaultBranch,
					Origin:        repo,
					Owner:         r.Owner.UserName,
					Hoster:        types.GetHost(repo.URL),
				})
				if r.HasWiki && repo.Wiki && types.StatRemote(r.CloneURL, r.SSHURL, repo) {
					repos = append(repos, types.Repo{
						Name:          r.Name + ".wiki",
						URL:           types.DotGitRx.ReplaceAllString(r.CloneURL, ".wiki.git"),
						SSHURL:        types.DotGitRx.ReplaceAllString(r.SSHURL, ".wiki.git"),
						Token:         token,
						Defaultbranch: r.DefaultBranch,
						Origin:        repo,
						Owner:         r.Owner.UserName,
						Hoster:        types.GetHost(repo.URL),
					})
				}
			}
		}
	}

	return repos, ran
}

func getOrgRepos(client *gitea.Client, org *gitea.Organization,
	orgopt gitea.ListOptions, repo types.GenRepo,
) []*gitea.Repository {
	o, _, err := client.ListOrgRepos(org.UserName,
		gitea.ListOrgReposOptions{orgopt})
	if err != nil {
		log.Error().Str("stage", "gitea").Str("url", repo.URL).Msg(err.Error())
	}

	return o
}
