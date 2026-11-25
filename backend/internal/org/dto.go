package org

type CreateOrgRequest struct {
	Name string `json:"name" binding:"required"`
}
