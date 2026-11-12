package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strconv"
	"sync"
	"time"
)

type RobloxClient struct {
	client    *http.Client
	csrfToken string
	csrfMu    sync.RWMutex
}

func NewRobloxClient() *RobloxClient {
	return &RobloxClient{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (rc *RobloxClient) doRequest(ctx context.Context, method, endpoint string, body []byte, cookie string) (*http.Response, error) {
	makeReq := func(token string) (*http.Request, error) {
		var bodyReader io.Reader
		if body != nil {
			bodyReader = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, endpoint, bodyReader)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "RoSniper-Go/2.0 (Windows NT 10.0; Win64; x64)")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		if cookie != "" {
			req.Header.Set("Cookie", fmt.Sprintf(".ROBLOSECURITY=%s", cookie))
		}
		if token != "" {
			req.Header.Set("X-CSRF-TOKEN", token)
		}
		return req, nil
	}

	rc.csrfMu.RLock()
	token := rc.csrfToken
	rc.csrfMu.RUnlock()

	req, err := makeReq(token)
	if err != nil {
		return nil, err
	}

	resp, err := rc.client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusForbidden {
		newToken := resp.Header.Get("X-CSRF-TOKEN")
		if newToken != "" {
			resp.Body.Close()
			rc.csrfMu.Lock()
			rc.csrfToken = newToken
			rc.csrfMu.Unlock()

			retryReq, err := makeReq(newToken)
			if err != nil {
				return nil, err
			}
			return rc.client.Do(retryReq)
		}
	}
	return resp, nil
}

type AuthenticatedUserResponse struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
}

func (rc *RobloxClient) ValidateCookie(ctx context.Context, cookie string) (*AuthenticatedUserResponse, error) {
	resp, err := rc.doRequest(ctx, http.MethodGet, "https://users.roblox.com/v1/users/authenticated", nil, cookie)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("invalid cookie (status: %d)", resp.StatusCode)
	}

	var user AuthenticatedUserResponse
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, err
	}
	return &user, nil
}

func (rc *RobloxClient) ResolveUsername(ctx context.Context, username string) (int64, string, error) {
	reqBody, _ := json.Marshal(map[string]interface{}{
		"usernames":          []string{username},
		"excludeBannedUsers": false,
	})

	resp, err := rc.doRequest(ctx, http.MethodPost, "https://users.roblox.com/v1/usernames/users", reqBody, "")
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()

	var result struct {
		Data []struct {
			ID          int64  `json:"id"`
			Name        string `json:"name"`
			DisplayName string `json:"displayName"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, "", err
	}
	if len(result.Data) == 0 {
		return 0, "", fmt.Errorf("user '%s' not found", username)
	}

	userID := result.Data[0].ID

	displayName, modernName, err := rc.GetUserProfileModern(ctx, userID)
	if err == nil && displayName != "" {
		return userID, fmt.Sprintf("%s (@%s)", displayName, modernName), nil
	}
	return userID, fmt.Sprintf("%s (@%s)", result.Data[0].DisplayName, result.Data[0].Name), nil
}

func (rc *RobloxClient) GetUserProfileModern(ctx context.Context, userID int64) (string, string, error) {
	reqBody, _ := json.Marshal(map[string]interface{}{
		"userIds": []int64{userID},
		"fields":  []string{"names.combinedName", "names.username", "names.displayName"},
	})

	resp, err := rc.doRequest(ctx, http.MethodPost, "https://apis.roblox.com/user-profile-api/v1/user/profiles/get-profiles", reqBody, "")
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("status: %d", resp.StatusCode)
	}

	var res struct {
		ProfileDetails []struct {
			Names struct {
				Username    string `json:"username"`
				DisplayName string `json:"displayName"`
			} `json:"names"`
		} `json:"profileDetails"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil || len(res.ProfileDetails) == 0 {
		return "", "", fmt.Errorf("parse error")
	}

	return res.ProfileDetails[0].Names.DisplayName, res.ProfileDetails[0].Names.Username, nil
}

type UserPresence struct {
	UserID           int64  `json:"userId"`
	UserPresenceType int    `json:"userPresenceType"`
	LastLocation     string `json:"lastLocation"`
	PlaceID          int64  `json:"placeId"`
	RootPlaceID      int64  `json:"rootPlaceId"`
	GameID           string `json:"gameId"`
	LastOnline       string `json:"lastOnline"`
}

func (rc *RobloxClient) GetPresence(ctx context.Context, userIDs []int64, cookie string) ([]UserPresence, error) {
	reqBody, _ := json.Marshal(map[string]interface{}{
		"userIds": userIDs,
	})

	resp, err := rc.doRequest(ctx, http.MethodPost, "https://presence.roblox.com/v1/presence/users", reqBody, cookie)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		retryAfter := 5
		if val := resp.Header.Get("Retry-After"); val != "" {
			if sec, err := strconv.Atoi(val); err == nil {
				retryAfter = sec
			}
		}
		return nil, fmt.Errorf("RATE_LIMITED:%d", retryAfter)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, string(body))
	}

	var presResp struct {
		UserPresences []UserPresence `json:"userPresences"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&presResp); err != nil {
		return nil, err
	}
	return presResp.UserPresences, nil
}

func (rc *RobloxClient) GetAuthTicket(ctx context.Context, cookie string) (string, error) {
	resp, err := rc.doRequest(ctx, http.MethodPost, "https://auth.roblox.com/v1/authentication-ticket", []byte("{}"), cookie)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	ticket := resp.Header.Get("RBX-Authentication-Ticket")
	if ticket != "" {
		return ticket, nil
	}

	var res struct {
		AuthenticationTicket string `json:"authenticationTicket"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err == nil && res.AuthenticationTicket != "" {
		return res.AuthenticationTicket, nil
	}
	return "", fmt.Errorf("ticket generation failed (%d)", resp.StatusCode)
}

func LaunchRoblox(placeID int64, gameID string, authTicket string) error {
	var launchURI string

	if authTicket != "" {
		placeLauncherURL := fmt.Sprintf(
			"https://assetgame.roblox.com/game/PlaceLauncher.ashx?request=RequestGameJob&placeId=%d&gameId=%s&isPlayTogetherGame=false",
			placeID, gameID,
		)
		launchURI = fmt.Sprintf(
			"roblox-player:1+launchmode:play+gameinfo:%s+launchtime:%d+placelauncherurl:%s+robloxLocale:en_us+gameLocale:en_us",
			url.QueryEscape(authTicket),
			time.Now().UnixMilli(),
			url.QueryEscape(placeLauncherURL),
		)
	} else {
		launchURI = fmt.Sprintf("roblox://experiences/start?placeId=%d&gameInstanceId=%s", placeID, gameID)
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", launchURI)
	case "darwin":
		cmd = exec.Command("open", launchURI)
	case "linux":
		cmd = exec.Command("xdg-open", launchURI)
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}

	return cmd.Start()
}
