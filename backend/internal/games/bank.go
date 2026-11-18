package games

// Shared, versioned question banks. Each entry freezes a canonical answer for
// reproducibility; time-sensitive items say so in the prompt.

type FermiBankQ struct {
	ID          string  `json:"id"`
	Prompt      string  `json:"prompt"`
	Units       string  `json:"units"`
	Answer      float64 `json:"answer"`
	Explanation string  `json:"explanation"`
	Source      string  `json:"source"`
}

var fermiBank = []FermiBankQ{
	{"f01", "Mass of planet Earth", "kg", 5.97e24, "Earth's mass is about 5.97 x 10^24 kg, derived from the gravitational constant and measured surface gravity.", "NASA planetary fact sheet"},
	{"f02", "Average distance from the Earth to the Moon", "km", 384400, "The mean Earth-Moon distance is 384,400 km.", "NASA"},
	{"f03", "Number of bones in the adult human body", "bones", 206, "The adult skeleton has 206 bones; babies have more, which fuse over time.", "standard anatomy references"},
	{"f04", "Speed of light in vacuum", "km/s", 299792.458, "c = 299,792.458 km/s exactly by SI definition.", "SI brochure"},
	{"f05", "World population (frozen value, November 2026)", "people", 8.2e9, "UN projections put world population at about 8.2 billion in 2025-2026.", "UN World Population Prospects"},
	{"f06", "Height of Mount Everest above sea level", "m", 8849, "The 2020 China-Nepal survey measured 8,848.86 m.", "2020 joint survey"},
	{"f07", "Number of seconds in a 365-day year", "s", 31536000, "365 x 24 x 3600 = 31,536,000.", "arithmetic"},
	{"f08", "Length of the Great Wall of China, all dynasties (official survey)", "km", 21196, "The 2012 heritage survey reports 21,196 km of walls, trenches and natural barriers.", "2012 SACH survey"},
	{"f09", "Number of United Nations member countries", "countries", 193, "The UN has 193 member states.", "UN member list"},
	{"f10", "Approximate number of stars in the Milky Way", "stars", 2e11, "Estimates range from 100 to 400 billion; the canonical mid value is 2 x 10^11.", "astronomy literature"},
	{"f11", "Depth of the Challenger Deep in the Mariana Trench", "m", 10935, "Modern surveys put the deepest point near 10,935 m.", "NOAA / 2019 expedition"},
	{"f12", "Age of the universe", "years", 1.38e10, "CMB measurements give 13.8 billion years.", "Planck mission results"},
	{"f13", "Approximate number of neurons in the adult human brain", "neurons", 8.6e10, "Isotropic fractionator counts give about 86 billion neurons.", "Herculano-Houzel 2009"},
	{"f14", "Length of the Amazon River", "km", 6400, "Commonly cited at about 6,400 km; some studies claim longer courses.", "geography references"},
	{"f15", "Total land area of Russia", "km^2", 1.71e7, "Russia covers about 17.1 million km^2.", "CIA World Factbook"},
	{"f16", "Number of works in the original Koechel catalog of Mozart", "works", 626, "The first edition Koechel catalog lists 626 compositions.", "Koechel catalog (1862)"},
	{"f17", "DNA base pairs in one human haploid genome", "base pairs", 3.2e9, "The haploid human genome is about 3.2 billion base pairs.", "Human Genome Project"},
	{"f18", "Speed of sound in air at 20 degrees C", "m/s", 343, "v = 343 m/s in dry air at 20 C.", "physics references"},
	{"f19", "Average distance from the Earth to the Sun", "km", 1.496e8, "One astronomical unit is 149,597,870 km.", "IAU definition"},
	{"f20", "Number of tiles in a standard English Scrabble set", "tiles", 100, "A standard set contains 100 letter tiles.", "Scrabble rules"},
	{"f21", "Water surface area of the Pacific Ocean", "km^2", 1.65e8, "About 165 million square kilometers.", "oceanography references"},
	{"f22", "Boiling point of water at sea level", "C", 100, "At 1 atm water boils at exactly 100 C on the Celsius scale.", "physics references"},
	{"f23", "Number of islands in Indonesia (official count)", "islands", 17000, "Indonesia counts over 17,000 islands (17,508 in common references).", "Indonesian government"},
	{"f24", "Mass of the Sun", "kg", 1.989e30, "The Sun's mass is about 1.989 x 10^30 kg.", "NASA fact sheet"},
}

type MarketQ struct {
	ID          string  `json:"id"`
	Prompt      string  `json:"prompt"`
	Units       string  `json:"units"`
	Answer      float64 `json:"answer"` // frozen, evaluator-only until reveal
	DomainUB    float64 `json:"domain_ub"` // public settlement domain [0, U]
	Explanation string  `json:"explanation"`
}

var marketBank = []MarketQ{
	{"m01", "Number of keys on a standard full-size piano", "keys", 88, 200, "A full-size piano has 88 keys (52 white, 36 black)."},
	{"m02", "Height of the Eiffel Tower including antennas", "m", 330, 1000, "The tower is 330 m tall with antennas (312 m without)."},
	{"m03", "Number of member clubs in the English Premier League", "clubs", 20, 100, "The Premier League has 20 member clubs."},
	{"m04", "Boiling point of water on the Fahrenheit scale at sea level", "F", 212, 1000, "Water boils at 212 F at 1 atm."},
	{"m05", "Number of countries in the European Union", "countries", 27, 100, "The EU has 27 member states after Brexit."},
	{"m06", "Length of a marathon race", "km", 42.195, 200, "The marathon distance is 42.195 km."},
	{"m07", "Number of stripes on the flag of the United States", "stripes", 13, 100, "The US flag has 13 stripes for the original colonies."},
	{"m08", "Freezing point of pure ethanol", "C", -114.1, 200, "Ethanol freezes at about -114.1 C."},
	{"m09", "Number of sonnets Shakespeare published", "sonnets", 154, 400, "Shakespeare's sequence contains 154 sonnets."},
	{"m10", "Maximum depth of Lake Baikal", "m", 1642, 5000, "Lake Baikal reaches 1,642 m, the deepest lake on Earth."},
}
