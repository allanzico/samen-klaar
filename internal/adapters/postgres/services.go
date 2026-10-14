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
	 ))`, "$AREA", parameter)
}

// Group identical scopes before AI selection: matching cost depends on catalog
// variety, not on how many professionals publish each service. Limit model input.
func (s *Store) ServiceCatalog(ctx context.Context, category, description, exclude string, locations ...workflow.Location) ([]matching.Service, error) {
	area := ""
	if len(locations) > 0 {
		area = locations[0].Label
	}
	rows, err := s.pool.Query(ctx, `SELECT p.service_name, p.service_description, p.materials_policy, p.exclusions_json
 FROM professional_profiles p JOIN account_memberships m ON m.account_id=p.account_id AND m.role='professional'
 WHERE p.status='active' AND m.status='active' AND p.category=$1 AND p.account_id<>$2
 AND `+areaCoverageSQL("$4")+`
 GROUP BY p.service_name, p.service_description, p.materials_policy, p.exclusions_json
 ORDER BY ts_rank(to_tsvector('english',p.service_name || ' ' || p.service_description), plainto_tsquery('english',$3)) DESC,
 p.service_name,p.service_description,p.materials_policy,p.exclusions_json LIMIT 200`, category, exclude, description, area)
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
	area := ""
	if len(locations) > 0 {
		area = locations[0].Label
	}
	rows, err := s.pool.Query(ctx, `SELECT p.id,p.account_id,p.display_name,p.category,p.service_name,p.service_description,p.service_area,
 p.currency,p.price_minor+p.callout_minor,p.duration_minutes,p.materials_policy,p.exclusions_json,
 earliest.id,earliest.starts_at,earliest.ends_at,earliest.timezone
 FROM professional_profiles p JOIN account_memberships m ON m.account_id=p.account_id AND m.role='professional'
 LEFT JOIN LATERAL (
 SELECT a.id,a.starts_at,a.ends_at,a.timezone FROM professional_availability_slots a
 WHERE a.professional_account_id=p.account_id AND a.status='available' AND a.starts_at>now()
 AND a.ends_at-a.starts_at >= make_interval(mins=>p.duration_minutes)
 AND NOT EXISTS (SELECT 1 FROM booking_reservations b WHERE b.professional_id=p.account_id AND b.status='booked'
 AND b.starts_at<a.ends_at AND b.ends_at>a.starts_at)
 ORDER BY a.starts_at,a.id LIMIT 1) earliest ON true
 WHERE p.status='active' AND m.status='active' AND p.category=$1 AND p.account_id<>$2
 AND p.service_name=$3 AND p.service_description=$4 AND p.materials_policy=$5 AND p.exclusions_json=$6::jsonb
	 AND `+areaCoverageSQL("$7")+`
	 ORDER BY earliest.starts_at NULLS LAST,p.id LIMIT 200`, category, exclude, service.Name, service.Description, service.Materials, exclusions, area)
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
		if err := rows.Scan(&match.ID, &match.AccountID, &match.Name, &match.Category, &match.ServiceName, &match.ServiceDescription, &match.Area,
			&match.Currency, &match.TotalMinor, &match.DurationMinutes, &match.MaterialsPolicy, &exclusions, &slot, &start, &end, &zone); err != nil {
			return nil, err
		}
		if !serviceAreaCovers(match.Area, area) {
			continue
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

// Provider areas are comma, semicolon, or newline separated neighbourhoods
// and postcode prefixes. Matching uses these published labels; it does not
// claim geocoding or travel-time precision.
func serviceAreaCovers(providerAreas, requestedArea string) bool {
	requested := normalizeArea(requestedArea)
	if requested == "" {
		return true
	}
	if !workflow.IsAmsterdamServiceArea(requestedArea) {
		return false
	}
	requestedPostcode := postcodePrefix(requestedArea)
	requestedAmsterdam := strings.HasPrefix(requested, "amsterdam ") || strings.HasPrefix(requested, "amsterdam-") || strings.HasPrefix(requested, "amsterdam/")
	requestedNeighborhood := stripAmsterdam(requested)
	for _, raw := range strings.FieldsFunc(providerAreas, func(r rune) bool { return r == ',' || r == ';' || r == '\n' || r == '|' }) {
		provider := normalizeArea(raw)
		if provider == "" {
			continue
		}
		providerPostcode := postcodePrefix(raw)
		if requestedPostcode != "" {
			if providerPostcode != "" && providerPostcode == requestedPostcode {
				return true
			}
			if provider == "amsterdam" && strings.HasPrefix(requestedPostcode, "10") {
				return true
			}
			continue
		}
		if provider == "amsterdam" && (requestedAmsterdam || isKnownAmsterdamArea(requestedNeighborhood)) {
			return true
		}
		if stripAmsterdam(provider) == requestedNeighborhood {
			return true
		}
	}
	return false
}

func isKnownAmsterdamArea(value string) bool {
	switch value {
	case "centrum", "noord", "zuid", "oost", "west", "nieuw-west", "zuidoost", "de pijp", "jordaan", "oud-west", "oud-zuid", "ijburg", "watergraafsmeer", "bos en lommer", "slotervaart", "zeeburg", "indische buurt", "osdorp":
		return true
	default:
		return false
	}
}

func normalizeArea(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.NewReplacer("’", "'", "–", "-", "—", "-").Replace(value)
	return strings.Join(strings.Fields(value), " ")
}

func stripAmsterdam(value string) string {
	value = normalizeArea(value)
	value = strings.TrimPrefix(value, "amsterdam ")
	value = strings.TrimPrefix(value, "amsterdam-")
	value = strings.TrimPrefix(value, "amsterdam/")
	return strings.TrimSpace(value)
}

func postcodePrefix(value string) string {
	value = strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(value)), "AMSTERDAM ")
	var compact strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(value)) {
		if r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' {
			compact.WriteRune(r)
		}
	}
	code := compact.String()
	if len(code) == 4 && isDigits(code) {
		return code
	}
	if len(code) == 6 && isDigits(code[:4]) && isLetters(code[4:]) {
		return code[:4]
	}
	return ""
}

func isDigits(value string) bool {
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return value != ""
}

func isLetters(value string) bool {
	for _, r := range value {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return value != ""
}
