package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/allanzico/samen-klaar/internal/workflow"
	"github.com/jackc/pgx/v5"
)

func locationJSON(locations []workflow.Location) []byte {
	l := workflow.Location{}
	if len(locations) > 0 {
		l = workflow.DiscoveryLocation(locations[0])
	}
	data, _ := json.Marshal(l)
	return data
}

// Area searches use the closest point of the geocoder's bounding box, not just
// the city centre. Actual booking always checks the full street address.
func locationDistanceSQL(parameter string, area bool) string {
	lat := "(" + parameter + "::jsonb->>'latitude')::double precision"
	lon := "(" + parameter + "::jsonb->>'longitude')::double precision"
	if area {
		lat = "CASE WHEN " + parameter + "::jsonb->>'kind'<>'address' AND jsonb_array_length(COALESCE(" + parameter + "::jsonb->'bounds','[]'::jsonb))=4 THEN greatest((" + parameter + "::jsonb#>>'{bounds,1}')::double precision,least(p.latitude,(" + parameter + "::jsonb#>>'{bounds,3}')::double precision)) ELSE " + lat + " END"
		lon = "CASE WHEN " + parameter + "::jsonb->>'kind'<>'address' AND jsonb_array_length(COALESCE(" + parameter + "::jsonb->'bounds','[]'::jsonb))=4 THEN greatest((" + parameter + "::jsonb#>>'{bounds,0}')::double precision,least(p.longitude,(" + parameter + "::jsonb#>>'{bounds,2}')::double precision)) ELSE " + lon + " END"
	}
	return "6371.0 * acos(least(1.0,greatest(-1.0,sin(radians(p.latitude))*sin(radians(" + lat + "))+cos(radians(p.latitude))*cos(radians(" + lat + "))*cos(radians(p.longitude-(" + lon + "))))))"
}

func locationCoverageSQL(parameter string) string {
	label := "COALESCE(" + parameter + "::jsonb->>'label','')"
	legacy := areaCoverageSQL(label)
	// Profiles which predate geocoding keep their published coverage, without a fabricated distance.
	geoLegacy := "(" + legacy + " OR " + areaCoverageSQL("COALESCE(NULLIF("+parameter+"::jsonb->>'postcode',''),"+label+")") + " OR " + areaCoverageSQL("COALESCE(NULLIF("+parameter+"::jsonb->>'neighborhood',''),"+label+")") + ")"
	return "(CASE WHEN " + parameter + "::jsonb->>'source'='mapbox' THEN CASE WHEN p.base_location IS NOT NULL THEN (" + locationDistanceSQL(parameter, true) + ") <= p.travel_radius_km ELSE " + geoLegacy + " END ELSE " + legacy + " END)"
}

func validateVisitCoverageTx(ctx context.Context, tx pgx.Tx, job workflow.Job) error {
	var baseJSON []byte
	var radius int
	var areas string
	err := tx.QueryRow(ctx, `SELECT base_location,travel_radius_km,service_area FROM professional_profiles WHERE account_id=$1`, job.ProfessionalAccountID).Scan(&baseJSON, &radius, &areas)
	if err != nil {
		return err
	}
	if len(baseJSON) == 0 {
		if job.ServiceLocation != nil && !workflow.ServiceAreaCoversLocation(areas, *job.ServiceLocation) {
			return errors.New("the visit address is outside the professional’s published service area")
		}
		return nil
	}
	var base workflow.Location
	if json.Unmarshal(baseJSON, &base) != nil {
		return errors.New("professional service coverage is unavailable")
	}
	if job.ServiceLocation == nil || job.ServiceLocation.Kind != "address" || !workflow.CoversLocation(base, radius, *job.ServiceLocation) {
		return errors.New("confirm a full visit address inside the professional’s service area before booking")
	}
	return nil
}

func matchDistanceSQL(parameter string) string {
	return "CASE WHEN " + parameter + "::jsonb->>'source'='mapbox' AND p.base_location IS NOT NULL THEN " + locationDistanceSQL(parameter, false) + " ELSE NULL END"
}
