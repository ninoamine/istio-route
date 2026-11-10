package simulate

import "errors"

var (
	ErrWaypointNotFound = errors.New("waypoint not found")
	ErrNotWaypoint      = errors.New("gateway is not an Istio ambient waypoint")
	ErrRouteNotFound    = errors.New("HTTPRoute not found")
	ErrRouteIneligible  = errors.New("HTTPRoute is not attached to this waypoint")
)
