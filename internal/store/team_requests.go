package store

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"dossier/internal/core"
)

// AppendTeamJoinRequest appends a request to the normalized login's own shard.
func (s *FSStore) AppendTeamJoinRequest(request core.TeamJoinRequest) error {
	if request.Resolution != "" || !request.ResolvedAt.IsZero() {
		return fmt.Errorf("team join request cannot include resolution fields")
	}
	if request.ID == "" {
		id, err := newTeamJoinRequestID()
		if err != nil {
			return err
		}
		request.ID = id
	}
	return s.appendTeamJoinRecord(request)
}

// AppendTeamJoinResolution appends an approval or roster-add watermark without rewriting requests.
func (s *FSStore) AppendTeamJoinResolution(request core.TeamJoinRequest) error {
	approved := request.Resolution == core.TeamJoinRequestResolutionApproved && request.ID != "" && !request.RequestedAt.IsZero()
	rosterAdded := request.Resolution == core.TeamJoinRequestResolutionRosterAdded && request.ID == "" && request.RequestedAt.IsZero()
	if (!approved && !rosterAdded) || request.ResolvedAt.IsZero() {
		return fmt.Errorf("team join resolution requires a valid status and timestamp")
	}
	return s.appendTeamJoinRecord(request)
}

func (s *FSStore) appendTeamJoinRecord(request core.TeamJoinRequest) error {
	request.Username = core.NormalizeUsername(request.Username)
	if !validTeamJoinRequestRecord(request) {
		return fmt.Errorf("invalid team join request record for %q", request.Username)
	}
	dir := filepath.Join(s.dossierHome, "team-requests")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("marshal team join record: %w", err)
	}
	path := teamJoinRequestPath(dir, request.Username)
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open team join request log: %w", err)
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		_ = file.Close()
		return fmt.Errorf("append team join record: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close team join request log: %w", err)
	}
	return nil
}

// TeamJoinRequests reads the synced append-only request shards.
func (s *FSStore) TeamJoinRequests() ([]core.TeamJoinRequest, error) {
	paths, err := filepath.Glob(filepath.Join(s.dossierHome, "team-requests", "*.log"))
	if err != nil {
		return nil, err
	}
	var requests []core.TeamJoinRequest
	for _, path := range paths {
		shard := strings.TrimSuffix(filepath.Base(path), ".log")
		usernameBytes, err := base64.RawURLEncoding.DecodeString(shard)
		if err != nil {
			return nil, fmt.Errorf("decode team join request shard %q: %w", path, err)
		}
		shardUsername := core.NormalizeUsername(string(usernameBytes))
		file, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("open team join request log %q: %w", path, err)
		}
		scanner := bufio.NewScanner(file)
		line := 0
		for scanner.Scan() {
			line++
			if len(scanner.Bytes()) == 0 {
				continue
			}
			var request core.TeamJoinRequest
			if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
				_ = file.Close()
				return nil, fmt.Errorf("parse team join request %s:%d: %w", path, line, err)
			}
			request.Username = core.NormalizeUsername(request.Username)
			if request.Username == "" || request.Username != shardUsername || !validTeamJoinRequestRecord(request) {
				_ = file.Close()
				return nil, fmt.Errorf("invalid team join request in %s:%d", path, line)
			}
			requests = append(requests, request)
		}
		if err := scanner.Err(); err != nil {
			_ = file.Close()
			return nil, fmt.Errorf("read team join request log %q: %w", path, err)
		}
		if err := file.Close(); err != nil {
			return nil, fmt.Errorf("close team join request log %q: %w", path, err)
		}
	}
	return requests, nil
}

func (s *FSStore) TeamJoinRequestSyncPath(username string) string {
	return filepath.ToSlash(filepath.Join("team-requests", teamJoinRequestShard(username)))
}

func teamJoinRequestPath(dir, username string) string {
	return filepath.Join(dir, teamJoinRequestShard(username))
}

func teamJoinRequestShard(username string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(core.NormalizeUsername(username))) + ".log"
}

func validTeamJoinRequestRecord(request core.TeamJoinRequest) bool {
	if request.Username == "" {
		return false
	}
	validID := len(request.ID) == 32
	if validID {
		_, err := hex.DecodeString(request.ID)
		validID = err == nil
	}
	switch request.Resolution {
	case "":
		return validID && !request.RequestedAt.IsZero() && request.ResolvedAt.IsZero()
	case core.TeamJoinRequestResolutionApproved:
		return validID && !request.RequestedAt.IsZero() && !request.ResolvedAt.IsZero()
	case core.TeamJoinRequestResolutionRosterAdded:
		return request.ID == "" && request.RequestedAt.IsZero() && !request.ResolvedAt.IsZero()
	default:
		return false
	}
}

func newTeamJoinRequestID() (string, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", fmt.Errorf("generate team join request ID: %w", err)
	}
	return hex.EncodeToString(id[:]), nil
}
