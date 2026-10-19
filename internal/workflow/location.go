package workflow

import (
	"math"
	"strings"
)

func ValidGeocodedLocation(l Location) bool {
	if l.Source != "mapbox" || l.ID == "" || l.Country != "nl" || l.Label == "" || math.IsNaN(l.Latitude) || math.IsNaN(l.Longitude) || math.IsInf(l.Latitude, 0) || math.IsInf(l.Longitude, 0) || l.Latitude < -90 || l.Latitude > 90 || l.Longitude < -180 || l.Longitude > 180 {
		return false
	}
	switch l.Kind {
	case "address", "postcode", "place", "locality", "neighborhood":
	default:
		return false
	}
	if len(l.Bounds) != 0 && len(l.Bounds) != 4 {
		return false
	}
	for _, value := range l.Bounds {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return false
		}
	}
	return len(l.Bounds) == 0 || l.Bounds[0] >= -180 && l.Bounds[2] <= 180 && l.Bounds[1] >= -90 && l.Bounds[3] <= 90 && l.Bounds[0] <= l.Longitude && l.Longitude <= l.Bounds[2] && l.Bounds[1] <= l.Latitude && l.Latitude <= l.Bounds[3]
}

// DiscoveryLocation keeps the exact address out of public labels and the shared thread.
func DiscoveryLocation(l Location) Location { l.Address = ""; l.Selection = ""; return l }

// CoversLocation uses an area's bounding box for broad searches. Only a full address
// establishes whether a specific visit is inside the professional's travel radius.
func CoversLocation(base Location, radius int, target Location) bool {
	if radius < 1 || !ValidGeocodedLocation(base) || !ValidGeocodedLocation(target) {
		return false
	}
	lat, lon := target.Latitude, target.Longitude
	if target.Kind != "address" && len(target.Bounds) == 4 {
		lon = math.Max(target.Bounds[0], math.Min(base.Longitude, target.Bounds[2]))
		lat = math.Max(target.Bounds[1], math.Min(base.Latitude, target.Bounds[3]))
	}
	return DistanceKM(base.Latitude, base.Longitude, lat, lon) <= float64(radius)
}

// Provider areas are comma, semicolon, or newline separated neighbourhoods
// and postcode prefixes. Matching uses these published labels; it does not
// claim geocoding or travel-time precision.
func PublishedAreaCovers(providerAreas, requestedArea string) bool {
	requested := normalizeArea(requestedArea)
	if requested == "" {
		return true
	}
	if !IsAmsterdamServiceArea(requestedArea) {
		return false
	}
	requestedPostcode := PostcodePrefix(requestedArea)
	requestedAmsterdam := strings.HasPrefix(requested, "amsterdam ") || strings.HasPrefix(requested, "amsterdam-") || strings.HasPrefix(requested, "amsterdam/")
	requestedNeighborhood := stripAmsterdam(requested)
	for _, raw := range strings.FieldsFunc(providerAreas, func(r rune) bool { return r == ',' || r == ';' || r == '\n' || r == '|' }) {
		provider := normalizeArea(raw)
		if provider == "" {
			continue
		}
		providerPostcode := PostcodePrefix(raw)
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

// ServiceAreaCoversLocation checks the published contract of a legacy profile
// against verified address components, rather than trusting a customer's search label.
func ServiceAreaCoversLocation(areas string, l Location) bool {
	if l.Source != "mapbox" {
		return PublishedAreaCovers(areas, l.Label)
	}
	if !ValidGeocodedLocation(l) {
		return false
	}
	if PublishedAreaCovers(areas, l.Label) || l.Postcode != "" && PublishedAreaCovers(areas, l.Postcode) || l.Neighborhood != "" && strings.EqualFold(l.City, "Amsterdam") && PublishedAreaCovers(areas, l.Neighborhood) {
		return true
	}
	for _, area := range strings.FieldsFunc(areas, func(r rune) bool { return r == ',' || r == ';' || r == '\n' || r == '|' }) {
		if strings.EqualFold(strings.TrimSpace(area), "Amsterdam") && strings.EqualFold(l.City, "Amsterdam") {
			return true
		}
	}
	return false
}

func OfferCoversLocation(offer CatalogOffer, l Location) bool {
	if offer.BaseLocation != nil {
		return CoversLocation(*offer.BaseLocation, offer.TravelRadiusKM, l)
	}
	return ServiceAreaCoversLocation(offer.ServiceArea, l)
}
