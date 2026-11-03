package repository

import "customer-support/internal/model"

// ListCauseCategories 원인분류(활성). §34.3.3 기존 공통 표. 수정하지 않는다.
func (r *ASRepo) ListCauseCategories() ([]model.CauseCategory, error) {
	return r.listLegacyCauseCategories(true)
}
