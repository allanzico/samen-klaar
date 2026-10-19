package postgres

import (
	"context"
	"encoding/json"
	"github.com/allanzico/samen-klaar/internal/matching"
	"github.com/allanzico/samen-klaar/internal/workflow"
	"strings"
	"time"
)

// Keep the catalog and final match query on the same SQL candidate filter.
// serviceAreaCovers remains the final Go check for the published area contract.
func areaCoverageSQL(parameter string) string {
	return strings.ReplaceAll(`($AREA = '' OR EXISTS (
		SELECT 1 FROM regexp_split_to_table(p.service_area, E'[,;|\\n]+') AS service_area(value)
		WHERE regexp_replace(lower(trim(service_area.value)), '^amsterdam[[:space:]/-]*', '') =
		      regexp_replace(lower(trim($AREA)), '^amsterdam[[:space:]/-]*', '')
		   OR lower(trim(service_area.value)) = 'amsterdam' AND (
				 lower(trim($AREA)) LIKE 'amsterdam %'
				 OR lower(trim($AREA)) LIKE 'amsterdam-%'
				 OR lower(trim($AREA)) LIKE 'amsterdam/%'
				 OR lower(trim($AREA)) IN ('centrum','noord','zuid','oost','west','nieuw-west','zuidoost','de pijp','jordaan','oud-west','oud-zuid','ijburg','watergraafsmeer','bos en lommer','slotervaart','zeeburg','indische buurt','osdorp')
				 OR regexp_replace(upper(trim($AREA)), '[^0-9]', '', 'g') ~ '^10[0-9]{2}$'
			)
		   OR left(regexp_replace(regexp_replace(upper(trim(service_area.value)), '^AMSTERDAM[[:space:]/-]*', ''), '[^A-Z0-9]', '', 'g'), 4) =
		      left(regexp_replace(upper(trim($AREA)), '[^A-Z0-9]', '', 'g'), 4)
			  AND left(regexp_replace(upper(trim($AREA)), '[^A-Z0-9]', '', 'g'), 4) ~ '^[0-9]{4}$'
		   OR left(regexp_replace(upper(trim($AREA)), '[^A-Z0-9]', '', 'g'), 4) ~ '^10[0-9]{2}$'
			  AND upper(trim(service_area.value)) ~ ('(^|[^A-Z0-9])' ||
				left(regexp_replace(upper(trim($AREA)), '[^A-Z0-9]', '', 'g'), 4) ||
				'([[:space:]-]*[A-Z]{2})?($|[^A-Z0-9])')
	 ))`, "$AREA", parameter)
}

// Group identical scopes before AI selection: matching cost depends on catalog
// variety, not on how many professionals publish each service. Limit model input.
func (s *Store) ServiceCatalog(ctx context.Context, category, description, exclude string, locations ...workflow.Location) ([]matching.Service, error) {
	location := locationJSON(locations)
	rows, err := s.pool.Query(ctx, `SELECT p.service_name, p.service_description, p.materials_policy, p.exclusions_json
 FROM professional_profiles p JOIN account_memberships m ON m.account_id=p.account_id AND m.role='professional'
 WHERE p.status='active' AND m.status='active' AND p.category=$1 AND p.account_id<>$2
 AND `+locationCoverageSQL("$4")+`
 GROUP BY p.service_name, p.service_description, p.materials_policy, p.exclusions_json
 ORDER BY ts_rank(to_tsvector('english',p.service_name || ' ' || p.service_description), plainto_tsquery('english',$3)) DESC,
 p.service_name,p.service_description,p.materials_policy,p.exclusions_json LIMIT 200`, category, exclude, description, location)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	services := []matching.Service{}
	for rows.Next() {
		var service matching.Service
		var exclusions []byte
		if err := rows.Scan(&service.Name, &service.Description, &service.Materials, &exclusions); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(exclusions, &service.Exclusions); err != nil {
			return nil, err
		}
		services = append(services, service)
	}
	return services, rows.Err()
}

func (s *Store) ServiceMatches(ctx context.Context, category, exclude string, service matching.Service, locations ...workflow.Location) ([]workflow.Match, error) {
	exclusions, err := json.Marshal(service.Exclusions)
	if err != nil {
		return nil, err
	}
	location := locationJSON(locations)
	rows, err := s.pool.Query(ctx, `SELECT p.id,p.account_id,p.display_name,p.category,p.service_name,p.service_description,p.service_area,
 p.currency,p.price_minor+p.callout_minor,p.duration_minutes,p.materials_policy,p.exclusions_json,
 earliest.id,earliest.starts_at,earliest.ends_at,earliest.timezone, `+matchDistanceSQL("$7")+` AS distance_km
 FROM professional_profiles p JOIN account_memberships m ON m.account_id=p.account_id AND m.role='professional'
 LEFT JOIN LATERAL (
 SELECT a.id,a.starts_at,a.ends_at,a.timezone FROM professional_availability_slots a
 WHERE a.professional_account_id=p.account_id AND `+bookableAvailabilitySQL("a", "p", "")+`
 ORDER BY a.starts_at,a.id LIMIT 1) earliest ON true
 WHERE p.status='active' AND m.status='active' AND p.category=$1 AND p.account_id<>$2
 AND p.service_name=$3 AND p.service_description=$4 AND p.materials_policy=$5 AND p.exclusions_json=$6::jsonb
	 AND `+locationCoverageSQL("$7")+`
	 ORDER BY distance_km NULLS LAST,earliest.starts_at NULLS LAST,p.id LIMIT 200`, category, exclude, service.Name, service.Description, service.Materials, exclusions, location)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	matches := []workflow.Match{}
	for rows.Next() {
		var match workflow.Match
		var slot, zone *string
		var start, end *time.Time
		var exclusions []byte
		var distance *float64
		if err := rows.Scan(&match.ID, &match.AccountID, &match.Name, &match.Category, &match.ServiceName, &match.ServiceDescription, &match.Area,
			&match.Currency, &match.TotalMinor, &match.DurationMinutes, &match.MaterialsPolicy, &exclusions, &slot, &start, &end, &zone, &distance); err != nil {
			return nil, err
		}
		if distance != nil {
			match.DistanceKnown = true
			match.DistanceKM = *distance
			match.DistanceApproximate = len(locations) > 0 && locations[0].Kind != "address"
		}
		if err := json.Unmarshal(exclusions, &match.Exclusions); err != nil {
			return nil, err
		}
		if slot != nil && start != nil && end != nil && zone != nil {
			match.EarliestAppointment = &workflow.Appointment{SlotID: *slot, Timezone: *zone, StartUTC: start.UTC().Format(time.RFC3339), EndUTC: end.UTC().Format(time.RFC3339), StartsAt: start.In(loadTimezone(*zone)).Format("Mon, 2 Jan · 15:04") + "–" + end.In(loadTimezone(*zone)).Format("15:04")}
		}
		matches = append(matches, match)
	}
	return matches, rows.Err()
}

func serviceAreaCovers(providerAreas, requestedArea string) bool {
	return workflow.PublishedAreaCovers(providerAreas, requestedArea)
}
