package sun

import (
	"fmt"
	"math"
	"time"
)

// Data holds computed sunrise and sunset for a location and date.
type Data struct {
	Sunrise  time.Time
	Sunset   time.Time
	Daylight time.Duration
}

// Compute calculates sunrise and sunset for the given lat/lon on date d.
// Uses the NOAA simplified solar position algorithm (accurate to ~1 minute).
// Returns times in UTC; call .In(loc) on results to convert to local time.
func Compute(lat, lon float64, d time.Time) (*Data, error) {
	jd := julianDay(d)

	// Julian centuries from J2000.0
	t := (jd - 2451545.0) / 36525.0

	// Geometric mean longitude of the sun (degrees)
	l0 := math.Mod(280.46646+t*(36000.76983+t*0.0003032), 360)

	// Geometric mean anomaly of the sun (degrees)
	m := 357.52911 + t*(35999.05029-0.0001537*t)
	mRad := degToRad(m)

	// Equation of center
	c := math.Sin(mRad)*(1.914602-t*(0.004817+0.000014*t)) +
		math.Sin(2*mRad)*(0.019993-0.000101*t) +
		math.Sin(3*mRad)*0.000289

	// Sun's true longitude
	sunLon := l0 + c

	// Apparent longitude (correcting for aberration and nutation)
	omega := 125.04 - 1934.136*t
	lambda := sunLon - 0.00569 - 0.00478*math.Sin(degToRad(omega))
	lambdaRad := degToRad(lambda)

	// Mean obliquity of the ecliptic
	epsilon0 := 23.0 + 26.0/60.0 + 21.448/3600.0 -
		t*(46.8150/3600.0+t*(0.00059/3600.0-t*0.001813/3600.0))
	epsilonRad := degToRad(epsilon0 + 0.00256*math.Cos(degToRad(omega)))

	// Sun's declination
	declRad := math.Asin(math.Sin(epsilonRad) * math.Sin(lambdaRad))

	// Equation of time (minutes)
	y := math.Tan(epsilonRad/2) * math.Tan(epsilonRad/2)
	l0Rad := degToRad(l0)
	mRad2 := degToRad(m)
	eot := 4 * radToDeg(y*math.Sin(2*l0Rad)-
		2*0.016708634*math.Sin(mRad2)+
		4*0.016708634*y*math.Sin(mRad2)*math.Cos(2*l0Rad)-
		0.5*y*y*math.Sin(4*l0Rad)-
		1.25*0.016708634*0.016708634*math.Sin(2*mRad2))

	// Solar noon (fractional day, UTC)
	latRad := degToRad(lat)

	// Hour angle for sunrise/sunset (solar elevation = -0.833°)
	cosHA := (math.Cos(degToRad(90.833)) - math.Sin(latRad)*math.Sin(declRad)) /
		(math.Cos(latRad) * math.Cos(declRad))

	if cosHA < -1 || cosHA > 1 {
		return nil, fmt.Errorf("sun never rises or sets at lat=%.2f on this date", lat)
	}

	haDeg := radToDeg(math.Acos(cosHA))

	// Solar noon in minutes from midnight UTC
	solarNoonMin := 720 - 4*lon - eot

	sunriseMin := solarNoonMin - haDeg*4
	sunsetMin := solarNoonMin + haDeg*4

	base := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
	sunrise := base.Add(time.Duration(sunriseMin * float64(time.Minute)))
	sunset := base.Add(time.Duration(sunsetMin * float64(time.Minute)))

	return &Data{
		Sunrise:  sunrise,
		Sunset:   sunset,
		Daylight: sunset.Sub(sunrise),
	}, nil
}

// Format returns a compact single-line sunrise/sunset summary.
// Times are displayed in the Europe/London timezone.
func Format(label string, d *Data) string {
	loc, err := time.LoadLocation("Europe/London")
	if err != nil {
		loc = time.UTC
	}
	rise := d.Sunrise.In(loc)
	set := d.Sunset.In(loc)
	h := int(d.Daylight.Hours())
	m := int(d.Daylight.Minutes()) % 60
	return fmt.Sprintf("🌅 %s: Rise %02d:%02d, Set %02d:%02d · %dh %02dm daylight",
		label, rise.Hour(), rise.Minute(), set.Hour(), set.Minute(), h, m)
}

func julianDay(t time.Time) float64 {
	t = t.UTC()
	y, m, d := t.Year(), int(t.Month()), t.Day()
	if m <= 2 {
		y--
		m += 12
	}
	a := y / 100
	b := 2 - a + a/4
	return float64(int(365.25*float64(y+4716))) +
		float64(int(30.6001*float64(m+1))) +
		float64(d) + float64(b) - 1524.5
}

func degToRad(d float64) float64 { return d * math.Pi / 180 }
func radToDeg(r float64) float64 { return r * 180 / math.Pi }
