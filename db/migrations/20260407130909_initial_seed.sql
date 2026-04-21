-- +goose Up
-- +goose StatementBegin
INSERT INTO roles (role_name, organization_id, organization_type, slug, is_system) VALUES
('owner', NULL, 'platform', 'platform.owner', TRUE)
, ('super_admin', NULL, 'platform', 'platform.super_admin', TRUE)
, ('support_staff', NULL, 'platform', 'platform.support_staff', TRUE)
, ('owner', NULL, 'merchant', 'merchant.owner', TRUE)
, ('manager', NULL, 'merchant', 'merchant.manager', TRUE)
, ('staff', NULL, 'merchant', 'merchant.staff', TRUE)
, ('owner', NULL, 'individual', 'individual.owner', TRUE)
, ('owner', NULL, 'company', 'company.owner', TRUE)
, ('admin', NULL, 'company', 'comany.admin', TRUE)
, ('buyer', NULL, 'company', 'company.buyer', TRUE)
, ('finance', NULL, 'company', 'company.finance', TRUE)
ON CONFLICT DO NOTHING;

INSERT INTO organizations (name, slug, type, status)
VALUES ('Platform', 'platform', 'platform', 'active');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE organizations DISABLE TRIGGER trg_protect_platform_org;
DELETE FROM organizations
WHERE slug = 'platform';
ALTER TABLE organizations ENABLE TRIGGER trg_protect_platform_org;
DELETE FROM roles
WHERE is_system = TRUE;
-- +goose StatementEnd
