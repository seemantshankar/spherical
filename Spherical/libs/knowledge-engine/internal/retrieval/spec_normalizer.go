// Package retrieval provides spec name normalization for universal automobile support.
package retrieval

import (
	"strings"
)

// SpecNormalizer normalizes spec names and maps them to canonical categories.
type SpecNormalizer struct {
	categoryAliases map[string]string
	specAliases     map[string][]string // Canonical name -> variations
	categorySpecMap map[string][]string // Category -> possible specs
}

// NewSpecNormalizer creates a new spec normalizer with comprehensive mappings.
func NewSpecNormalizer() *SpecNormalizer {
	return &SpecNormalizer{
		categoryAliases: buildCategoryAliases(),
		specAliases:     buildSpecAliases(),
		categorySpecMap: buildCategorySpecMap(),
	}
}

// NormalizeSpecName maps variations to canonical names and returns alternatives.
func (n *SpecNormalizer) NormalizeSpecName(specName string) (canonical string, alternatives []string) {
	specNameLower := strings.ToLower(strings.TrimSpace(specName))
	
	// Check if we have a direct mapping
	for canonicalName, variations := range n.specAliases {
		canonicalLower := strings.ToLower(canonicalName)
		if canonicalLower == specNameLower {
			return canonicalName, variations
		}
		// Check if specName matches any variation
		for _, variation := range variations {
			if strings.ToLower(variation) == specNameLower {
				return canonicalName, variations
			}
		}
	}
	
	// If no mapping found, return normalized version of input
	// Capitalize first letter of each word
	words := strings.Fields(specName)
	normalized := make([]string, len(words))
	for i, word := range words {
		if len(word) > 0 {
			normalized[i] = strings.ToUpper(word[:1]) + strings.ToLower(word[1:])
		}
	}
	canonical = strings.Join(normalized, " ")
	
	// Try to find alternatives based on category
	category := n.FindCanonicalCategory(specName)
	if specs, ok := n.categorySpecMap[category]; ok {
		alternatives = specs
	}
	
	return canonical, alternatives
}

// FindCanonicalCategory maps spec to category.
func (n *SpecNormalizer) FindCanonicalCategory(specName string) string {
	specNameLower := strings.ToLower(strings.TrimSpace(specName))
	
	// Direct category mapping for common specs
	categoryMappings := map[string]string{
		"fuel economy":      "Fuel Efficiency",
		"mileage":           "Fuel Efficiency",
		"fuel consumption":  "Fuel Efficiency",
		"fuel efficiency":   "Fuel Efficiency",
		"km/l":              "Fuel Efficiency",
		"kmpl":              "Fuel Efficiency",
		"mpg":               "Fuel Efficiency",
		"ground clearance":  "Dimensions",
		"ground clearance height": "Dimensions",
		"minimum ground clearance": "Dimensions",
		"engine torque":     "Engine",
		"torque":            "Engine",
		"maximum torque":    "Engine",
		"peak torque":       "Engine",
		"engine specifications": "Engine",
		"engine specs":      "Engine",
		"interior comfort":  "Comfort",
		"comfort features":  "Comfort",
		"seating comfort":   "Comfort",
		"suspension":        "Suspension",
		"fuel tank capacity": "Fuel Efficiency",
		"fuel tank":         "Fuel Efficiency",
	}
	
	if category, ok := categoryMappings[specNameLower]; ok {
		return category
	}
	
	// Check category aliases
	if category, ok := n.categoryAliases[specNameLower]; ok {
		return category
	}
	
	// Try partial matching
	for key, category := range categoryMappings {
		if strings.Contains(specNameLower, key) {
			return category
		}
	}
	
	// Default to "General" if no match found
	return "General"
}

// buildCategoryAliases builds the category alias map.
func buildCategoryAliases() map[string]string {
	return map[string]string{
		"engine specs":     "Engine",
		"engine":           "Engine",
		"fuel economy":     "Fuel Efficiency",
		"fuel efficiency":  "Fuel Efficiency",
		"mileage":          "Fuel Efficiency",
		"transmission":     "Transmission",
		"gearbox":          "Transmission",
		"dimensions":       "Dimensions",
		"size":             "Dimensions",
		"weight":           "Weight",
		"mass":             "Weight",
		"safety":           "Safety",
		"security":         "Safety",
		"comfort":          "Comfort",
		"interior":         "Comfort",
		"technology":       "Technology",
		"tech":             "Technology",
		"infotainment":     "Technology",
		"exterior":         "Exterior",
		"design":           "Exterior",
		"warranty":         "Warranty",
		"ground clearance": "Dimensions",
		"fuel consumption": "Fuel Efficiency",
		"interior comfort": "Comfort",
		"engine specifications": "Engine",
		"engine torque":    "Engine",
		"suspension":       "Suspension",
		"brakes":           "Brakes",
		"brake system":     "Brakes",
		"wheels":           "Wheels",
		"tires":            "Wheels",
		"tyres":            "Wheels",
		"seating":          "Comfort",
		"seats":            "Comfort",
		"upholstery":       "Comfort",
		"climate control":  "Comfort",
		"air conditioning": "Comfort",
		"ac":               "Comfort",
		"heating":          "Comfort",
		"ventilation":      "Comfort",
		"audio":            "Technology",
		"sound system":     "Technology",
		"speaker":          "Technology",
		"speakers":         "Technology",
		"navigation":       "Technology",
		"gps":              "Technology",
		"connectivity":     "Technology",
		"bluetooth":        "Technology",
		"usb":              "Technology",
		"wireless":         "Technology",
		"wireless charging": "Technology",
		"power":            "Engine",
		"horsepower":       "Engine",
		"hp":               "Engine",
		"bhp":              "Engine",
		"displacement":     "Engine",
		"engine capacity":  "Engine",
		"cc":               "Engine",
		"cylinders":        "Engine",
		"compression ratio": "Engine",
		"acceleration":     "Performance",
		"top speed":        "Performance",
		"max speed":        "Performance",
		"driving range":    "Fuel Efficiency",
		"battery capacity": "Fuel Efficiency",
		"charging":         "Fuel Efficiency",
		"electric range":  "Fuel Efficiency",
		"hybrid":           "Fuel Efficiency",
		"electric":         "Fuel Efficiency",
		"ev":               "Fuel Efficiency",
		"phev":             "Fuel Efficiency",
		"length":           "Dimensions",
		"width":            "Dimensions",
		"height":           "Dimensions",
		"wheelbase":        "Dimensions",
		"turning radius":   "Dimensions",
		"boot space":       "Dimensions",
		"trunk space":      "Dimensions",
		"luggage capacity": "Dimensions",
		"cargo space":      "Dimensions",
		"seating capacity":  "Comfort",
		"passenger capacity": "Comfort",
		"airbags":          "Safety",
		"airbag":           "Safety",
		"abs":              "Safety",
		"anti-lock braking": "Safety",
		"esc":              "Safety",
		"electronic stability": "Safety",
		"traction control":  "Safety",
		"parking sensors":   "Safety",
		"rear camera":       "Safety",
		"backup camera":     "Safety",
		"blind spot":        "Safety",
		"lane assist":      "Safety",
		"lane keeping":     "Safety",
		"adaptive cruise":  "Safety",
		"cruise control":   "Safety",
		"collision warning": "Safety",
		"automatic emergency braking": "Safety",
		"aeb":              "Safety",
		"child safety":     "Safety",
		"isofix":           "Safety",
		"isofix seats":     "Safety",
		"child seat":       "Safety",
		"headlights":       "Exterior",
		"headlamps":        "Exterior",
		"taillights":       "Exterior",
		"fog lights":       "Exterior",
		"led":              "Exterior",
		"sunroof":          "Exterior",
		"moonroof":         "Exterior",
		"panoramic roof":   "Exterior",
		"alloy wheels":     "Wheels",
		"steel wheels":     "Wheels",
		"wheel size":       "Wheels",
		"rim size":         "Wheels",
		"tire size":        "Wheels",
		"tyre size":        "Wheels",
		"colors":           "Exterior",
		"colours":          "Exterior",
		"color options":    "Exterior",
		"colour options":   "Exterior",
		"paint":            "Exterior",
		"body color":       "Exterior",
		"body colour":      "Exterior",
		"exterior color":   "Exterior",
		"exterior colour":  "Exterior",
		"interior color":   "Comfort",
		"interior colour":  "Comfort",
		"material":         "Comfort",
		"materials":        "Comfort",
		"leather":          "Comfort",
		"fabric":           "Comfort",
		"upholstery material": "Comfort",
	}
}

// buildSpecAliases builds the spec alias map (canonical name -> variations).
func buildSpecAliases() map[string][]string {
	return map[string][]string{
		"Fuel Economy": {
			"Mileage",
			"Fuel Consumption",
			"Fuel Efficiency",
			"km/l",
			"kmpl",
			"mpg",
			"Kilometers per Liter",
			"Miles per Gallon",
		},
		"Engine Torque": {
			"Torque",
			"Maximum Torque",
			"Peak Torque",
			"Torque Output",
		},
		"Ground Clearance": {
			"Ground Clearance Height",
			"Minimum Ground Clearance",
			"Clearance",
			"Ride Height",
		},
		"Interior Comfort": {
			"Comfort Features",
			"Seating Comfort",
			"Comfort",
		},
		"Engine Specifications": {
			"Engine Specs",
			"Engine",
			"Motor Specifications",
		},
		"Fuel Tank Capacity": {
			"Fuel Tank",
			"Tank Capacity",
			"Fuel Capacity",
		},
		"Suspension": {
			"Suspension System",
			"Front Suspension",
			"Rear Suspension",
		},
		"Seating Capacity": {
			"Passenger Capacity",
			"Seats",
			"Number of Seats",
		},
		"Boot Space": {
			"Trunk Space",
			"Luggage Capacity",
			"Cargo Space",
			"Cargo Capacity",
		},
		"Wheelbase": {
			"Wheel Base",
			"Wheelbase Length",
		},
		"Turning Radius": {
			"Minimum Turning Radius",
			"Turning Circle",
		},
		"Airbags": {
			"Airbag",
			"Safety Airbags",
			"Number of Airbags",
		},
		"ABS": {
			"Anti-lock Braking System",
			"Anti Lock Braking",
		},
		"Parking Sensors": {
			"Park Assist",
			"Parking Assist",
			"Reverse Sensors",
		},
		"Rear Camera": {
			"Backup Camera",
			"Reverse Camera",
			"Rear View Camera",
		},
		"Lane Assist": {
			"Lane Keeping Assist",
			"Lane Departure Warning",
			"Lane Keeping",
		},
		"Adaptive Cruise Control": {
			"Adaptive Cruise",
			"ACC",
			"Smart Cruise Control",
		},
		"ISOFIX": {
			"ISOFIX Seats",
			"Child Seat Anchors",
			"LATCH",
		},
		"Headlights": {
			"Headlamps",
			"Front Lights",
		},
		"Sunroof": {
			"Moonroof",
			"Panoramic Sunroof",
			"Panoramic Roof",
		},
		"Alloy Wheels": {
			"Alloy Rims",
			"Aluminum Wheels",
		},
		"Colors": {
			"Colours",
			"Color Options",
			"Colour Options",
			"Paint Colors",
			"Paint Colours",
		},
		"Body Color": {
			"Body Colour",
			"Exterior Color",
			"Exterior Colour",
		},
		"Interior Color": {
			"Interior Colour",
			"Upholstery Color",
			"Upholstery Colour",
		},
		"Leather Upholstery": {
			"Leather",
			"Leather Seats",
			"Genuine Leather",
		},
		"Fabric Upholstery": {
			"Fabric",
			"Cloth Upholstery",
			"Fabric Seats",
		},
	}
}

// buildCategorySpecMap builds the category to specs map.
func buildCategorySpecMap() map[string][]string {
	return map[string][]string{
		"Fuel Efficiency": {
			"Fuel Economy",
			"Mileage",
			"Fuel Consumption",
			"Fuel Tank Capacity",
			"Driving Range",
			"Battery Capacity",
			"Electric Range",
		},
		"Engine": {
			"Engine Torque",
			"Engine Specifications",
			"Power",
			"Horsepower",
			"Displacement",
			"Engine Capacity",
			"Cylinders",
			"Compression Ratio",
		},
		"Dimensions": {
			"Ground Clearance",
			"Length",
			"Width",
			"Height",
			"Wheelbase",
			"Turning Radius",
			"Boot Space",
			"Luggage Capacity",
		},
		"Comfort": {
			"Interior Comfort",
			"Seating Capacity",
			"Seating",
			"Upholstery",
			"Climate Control",
			"Air Conditioning",
			"Heating",
			"Ventilation",
		},
		"Safety": {
			"Airbags",
			"ABS",
			"ESC",
			"Traction Control",
			"Parking Sensors",
			"Rear Camera",
			"Blind Spot",
			"Lane Assist",
			"Adaptive Cruise Control",
			"Collision Warning",
			"Automatic Emergency Braking",
			"Child Safety",
			"ISOFIX",
		},
		"Technology": {
			"Audio",
			"Sound System",
			"Speakers",
			"Navigation",
			"GPS",
			"Connectivity",
			"Bluetooth",
			"USB",
			"Wireless Charging",
		},
		"Exterior": {
			"Headlights",
			"Taillights",
			"Fog Lights",
			"Sunroof",
			"Colors",
			"Body Color",
		},
		"Wheels": {
			"Alloy Wheels",
			"Steel Wheels",
			"Wheel Size",
			"Rim Size",
			"Tire Size",
		},
		"Suspension": {
			"Suspension",
			"Front Suspension",
			"Rear Suspension",
		},
		"Brakes": {
			"Brakes",
			"Brake System",
			"ABS",
		},
		"Performance": {
			"Acceleration",
			"Top Speed",
			"Max Speed",
		},
	}
}



