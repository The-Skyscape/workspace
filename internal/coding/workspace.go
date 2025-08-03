package coding

import (
	"bytes"
	"cmp"
	_ "embed"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/The-Skyscape/devtools/pkg/authentication"
	"github.com/The-Skyscape/devtools/pkg/containers"
	"github.com/The-Skyscape/devtools/pkg/database"

	"github.com/pkg/errors"
)

func (*Workspace) Table() string { return "code_workspaces" }

type Workspace struct {
	repo *Repository

	database.Model
	Name   string
	Port   int
	Ready  bool
	RepoID string
}

func (w *Workspace) Repo() (*GitRepo, error) {
	return w.repo.repos.Get(w.RepoID)
}

func (r *Repository) NewWorkspace(name string, port int, repo *GitRepo) (*Workspace, error) {
	workspace := &Workspace{repo: r, Name: name, Port: port, Ready: false}
	if repo != nil {
		workspace.RepoID = repo.ID
	}

	workspace.Model = r.db.NewModel(name)
	return r.spaces.Insert(workspace)
}

func (w *Workspace) Service() *containers.Service {
	return &containers.Service{
		Host:    containers.Local(),
		Name:    w.Name,
		Image:   "codercom/code-server:latest",
		Command: "--auth none",

		Mounts: map[string]string{
			fmt.Sprintf("%s/workspaces/%s/.config:/home/coder/.config", database.DataDir(), w.Name): "/home/coder/.config",
			fmt.Sprintf("%s/workspaces/%s/project:/home/coder/project", database.DataDir(), w.Name): "/home/coder/project",
		},
		Ports: map[int]int{
			w.Port: w.Port,
		},
		Env: map[string]string{
			"PORT": strconv.Itoa(w.Port),
		},
	}
}

func (r *Repository) GetWorkspace(name string) (*Workspace, error) {
	w, err := r.spaces.Search(`WHERE Name = ?`, name)
	if err != nil || len(w) == 0 {
		return nil, cmp.Or(err, errors.New("workspace not found"))
	}

	w[0].repo = r
	return w[0], nil
}

func (r *Repository) Workspaces() ([]*Workspace, error) {
	ws, err := r.spaces.Search(``)
	for _, w := range ws {
		w.repo = r
	}
	return ws, err
}

//go:embed resources/prepare-workspace.sh
var prepareWorkspace string

//go:embed resources/setup-workspace.sh
var setupWorkspace string

//go:embed resources/clone-git-repo.sh
var cloneRepository string

func (w *Workspace) Start(u *authentication.User) error {
	s := w.Service()
	if s.IsRunning() {
		log.Printf("Workspace %s already running", w.Name)
		return nil
	}

	host := containers.Local()
	host.SetStdout(os.Stdout)
	host.SetStderr(os.Stderr)
	if err := host.Exec("bash", "-c", fmt.Sprintf(prepareWorkspace, database.DataDir(), w.Name)); err != nil {
		return errors.Wrap(err, "failed to prepare workspace ")
	}

	if err := host.Launch(s); err != nil {
		return errors.Wrap(err, "failed to start workspace ")
	}

	if err := host.Exec("bash", "-c", setupWorkspace); err != nil {
		return errors.Wrap(err, "failed to setup workspace: ")
	}

	if repo, err := w.Repo(); repo != nil && err == nil {
		if token, err := w.repo.NewAccessToken(time.Now().Add(100_000 * time.Hour)); err == nil {
			w.Run(fmt.Sprintf(cloneRepository, token.ID, token.Secret, w.RepoID, u.Name, u.Email))
		} else {
			log.Println("Failed to create access token:", err)
		}
	} else {
		log.Println("Failed to load repo:", repo, err)
	}

	w.Ready = true
	return w.repo.spaces.Update(w)
}

func (w *Workspace) Run(cmd string) (stdout bytes.Buffer, err error) {
	s := w.Service()

	s.SetStdout(&stdout)
	cmd = strings.ReplaceAll(cmd, "\n", "; ")
	cmd = strings.ReplaceAll(cmd, "; ;", ";")
	return stdout, s.Exec("docker", "exec", w.Name, "bash", "-c", cmd)
}
