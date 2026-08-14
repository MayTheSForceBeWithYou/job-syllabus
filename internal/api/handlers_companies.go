package api

import "net/http"

// handleListCompanies implements GET /v1/companies — the static registry
// from data/companies.yaml plus live posting counts. See companyDTO's
// comment for what's deliberately omitted.
func (s *Server) handleListCompanies(w http.ResponseWriter, r *http.Request) {
	postings, err := s.Store.ListAllPostings(r.Context())
	if err != nil {
		writeProblem(w, r, http.StatusInternalServerError, "failed to list postings", err.Error())
		return
	}

	counts := make(map[string]int, len(s.Companies))
	for _, p := range postings {
		counts[p.CompanySlug]++
	}

	resp := make([]companyDTO, 0, len(s.Companies))
	for _, c := range s.Companies {
		resp = append(resp, companyDTO{
			Slug:         c.Slug,
			Name:         c.Name,
			Tier:         c.Tier,
			ATS:          c.ATS,
			PostingCount: counts[c.Slug],
		})
	}

	writeJSON(w, listCompaniesResponse{Companies: resp})
}
