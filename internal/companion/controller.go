package companion

import "net/http"

// controllerState is what the Controller section of the Game tab shows: whether a pad is connected,
// which buttons are down (the game station's layout, one bit per button) and the stick positions.
// It is a page helper for this window, not part of the public API.
func (s *APIServer) controllerState(w http.ResponseWriter, _ *http.Request) {
	state := s.input.Live()
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"connected": state.Connected, "buttons": state.DigitalButtons, "axes": state.Axes})
}
