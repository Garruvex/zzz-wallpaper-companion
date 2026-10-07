package companion

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"
)

func companionResponding(port int) bool {
	client := &http.Client{Timeout: 1500 * time.Millisecond, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	response, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/api/health", port))
	if err != nil {
		return false
	}
	defer response.Body.Close()
	var health struct {
		Name     string `json:"name"`
		Protocol int    `json:"protocolVersion"`
	}
	return response.StatusCode == http.StatusOK && json.NewDecoder(http.MaxBytesReader(nil, response.Body, 16<<10)).Decode(&health) == nil && health.Name == "zzz-wallpaper-companion" && health.Protocol > 0
}

// Recovery listens separately for settings only; the wallpaper port never changes silently.
func (s *APIServer) prepareListener() (net.Listener, string, error) {
	port := s.config.Get().Port
	listener, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", port))
	if err == nil {
		return listener, "", nil
	}
	warning := fmt.Sprintf("Connection unavailable: port %d could not be opened. %v", port, err)
	if companionResponding(port) {
		warning += " Another ZZZ companion is responding on this port. Use that instance, or quit it before restarting this one."
	} else {
		warning += " Another application or an unresponsive companion may be using it. If a ZZZ companion is stuck, end only that companion process in Task Manager, then restart. Otherwise choose a free port below and set the same port in Wallpaper Engine's Companion App settings."
	}
	warning += " The companion remains open, but wallpaper services are unavailable until the connection is fixed. Save any port change and quit/restart the companion. Default port: 8765."
	listener, recoveryErr := net.Listen("tcp4", "127.0.0.1:0")
	if recoveryErr != nil {
		return nil, warning, fmt.Errorf("cannot open recovery settings: %w", recoveryErr)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /settings", func(w http.ResponseWriter, r *http.Request) { s.renderSettings(w, warning) })
	mux.HandleFunc("GET /api/settings", s.getSettings)
	mux.HandleFunc("POST /api/settings", s.saveSettings)
	s.http.Handler = localOnly(cors(mux))
	return listener, warning, nil
}
