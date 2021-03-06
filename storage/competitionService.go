package storage

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/sheitm/ofever/types"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	patternCompetitionLength = `\(\d{1,2}(.|,)\d{1} km\)`
	patternFloat = `\d{1,2}(.|,)\d{1}`
)

var (
	regexCompetitionLength = regexp.MustCompile(patternCompetitionLength)
	regexFloat = regexp.MustCompile(patternFloat)
)

type competitionService interface {
	Start(element seasonSyncElement)
	List() ([]*competition, error)
	CompetitionByNames(eventName, courseName string) (competitionAndCourse, error)
	//CompetitionsForSeason(season int) []*competition
}

func newCompetitionService(persist competitionPersistFunc, fetch competitionFetchFunc) competitionService {
	impl := &competitionServiceImpl{
		competitions: map[string]*competition{},
		persist:      persist,
	}
	impl.init(fetch)
	return impl
}

type competitionServiceImpl struct {
	competitions map[string]*competition
	persist      competitionPersistFunc
}

func (c *competitionServiceImpl) Start(element seasonSyncElement) {
	go func(sc <-chan *types.SeasonFetch, dc chan<- struct{}){
		for {
			fetch := <- sc
			anyChange := false
			if fetch.Results == nil {
				continue
			}
			for _, result := range fetch.Results {
				if result.Event == nil {
					continue
				}
				e := result.Event
				id := makeCompetitionID(e.Date)
				if _, ok := c.competitions[id]; ok {
					continue
				}
				anyChange = true
				comp := getCompetition(id, e)
				c.competitions[id] = comp
			}
			if anyChange {
				l, err := c.List()
				if err != nil {
					log.Printf("%v", err)
					continue
				}
				c.persist(l)
			}
			dc <- struct{}{}
		}
	}(element.seasonChan, element.doneChan)
}

func (c *competitionServiceImpl) List() ([]*competition, error) {
	var result []*competition
	for _, co := range c.competitions {
		result = append(result, co)
	}
	return result, nil
}

func (c *competitionServiceImpl) init(fetch competitionFetchFunc) {
	l, err := fetch()
	if err != nil {
		log.Printf("%v", err)
		return
	}
	for _, co := range l {
		c.competitions[co.ID] = co
	}
}

func (c *competitionServiceImpl) CompetitionByNames(eventName, courseName string) (competitionAndCourse, error) {
	for _, comp := range c.competitions {
		if comp.Name != eventName {
			continue
		}
		for _, crs := range comp.Courses {
			if crs.Name != courseName {
				continue
			}
			res :=  competitionAndCourse{
				competition: comp,
				course:      &crs,
			}
			return res, nil
		}
	}
	return competitionAndCourse{}, fmt.Errorf("could not find competition %s with course %s", eventName, courseName)
}

func (c *competitionServiceImpl) CompetitionsForSeason(season int) []*competition {
	s := fmt.Sprintf("%d", season)
	var result []*competition
	for k, comp := range c.competitions {
		if k[0:4] == s {
			result = append(result, comp)
		}
	}
	return result
}

func makeCompetitionID(dt time.Time) string {
	guid := uuid.New()
	return fmt.Sprintf("%d%02d%02d-%s", dt.Year(), dt.Month(), dt.Day(), guid.String()[0:4])
}

func getCompetition(id string, e *types.Event) *competition {
	comp := &competition{
		ID:          id,
		Number:      e.Number,
		Name:        e.Name,
		Date:        e.Date,
		Courses:     nil,
		Info:        e.Info,
		URL:         e.URL,
		URLInvite:   e.URLInvite,
		URLLiveLox:  e.URLLiveLox,
		Place:       e.Place,
		Organizer:   e.Organizer,
		Responsible: e.Responsible,
	}
	if e.Courses == nil {
		return comp
	}
	var courses []course
	for _, ec := range e.Courses {
		courseType := getCourseType(ec.Name)
		c := course{
			ID:         id + "-" + courseType,
			Name:       ec.Name,
			Info:       ec.Info,
			Length:     getCourseLength(ec.Name),
			CourseType: courseType,
		}
		courses = append(courses, c)
	}
	comp.Courses = courses
	return comp
}

func getCourseType(n string) string {
	l := strings.ToLower(n)
	if strings.Contains(l, "lang") {
		return "long"
	}
	if strings.Contains(l, "mellom") {
		return "medium"
	}
	if strings.Contains(l, "kort") {
		return "short"
	}
	return "newbie"
}

func getCourseLength(n string) float64 {
	r := regexCompetitionLength.Find([]byte(n))
	if r == nil || len(r) == 0 {
		return 0
	}
	r = regexFloat.Find(r)
	s := strings.ReplaceAll(string(r), ",", ".")
	res, _ := strconv.ParseFloat(s, 10)
	return res
}