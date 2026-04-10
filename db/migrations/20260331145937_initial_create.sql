-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS identities (
    id UUID NOT NULL PRIMARY KEY DEFAULT uuidv7()
    , type TEXT NOT NULL
    , created_at TIMESTAMPTZ NOT NULL DEFAULT now()
    , CONSTRAINT check_identity_type CHECK (type IN ('user', 'customer'))
);

CREATE TABLE IF NOT EXISTS users (
    id UUID NOT NULL PRIMARY KEY DEFAULT uuidv7()
    , identity_id UUID NOT NULL
    , name TEXT NOT NULL
    , email TEXT NOT NULL
    , email_verified BOOLEAN NOT NULL DEFAULT false
    , image TEXT DEFAULT null
    , created_at TIMESTAMPTZ NOT NULL DEFAULT now()
    , updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
    , CONSTRAINT fk_users_identity_id FOREIGN KEY (
        identity_id
    ) REFERENCES identities (id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS user_accounts (
    id UUID NOT NULL PRIMARY KEY DEFAULT uuidv7()
    , user_id UUID NOT NULL
    , account_id TEXT NOT NULL
    , provider_id TEXT NOT NULL
    , access_token TEXT
    , refresh_token TEXT
    , access_token_expires_at TIMESTAMPTZ
    , refresh_token_expires_at TIMESTAMPTZ
    , scope TEXT
    , id_token TEXT
    , hashed_password TEXT
    , created_at TIMESTAMPTZ NOT NULL DEFAULT now()
    , updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
    , CONSTRAINT fk_users_accounts_user_id FOREIGN KEY (
        user_id
    ) REFERENCES users (
        id
    ) ON DELETE CASCADE
    , CONSTRAINT check_user_accounts_provider_id CHECK (provider_id IN (
        'credential'
        , 'google'
    ))
);

CREATE TABLE IF NOT EXISTS customers (
    id UUID NOT NULL PRIMARY KEY DEFAULT uuidv7()
    , identity_id UUID NOT NULL
    , name TEXT NOT NULL
    , email TEXT NOT NULL
    , email_verified BOOLEAN NOT NULL DEFAULT false
    , image TEXT
    , created_at TIMESTAMPTZ NOT NULL DEFAULT now()
    , updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
    , CONSTRAINT fk_customers_identity_id FOREIGN KEY (
        identity_id
    ) REFERENCES identities (id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS customer_accounts (
    id UUID NOT NULL PRIMARY KEY DEFAULT uuidv7()
    , customer_id UUID NOT NULL
    , account_id TEXT NOT NULL
    , provider_id TEXT NOT NULL
    , access_token TEXT
    , refresh_token TEXT
    , access_token_expires_at TIMESTAMPTZ
    , refresh_token_expires_at TIMESTAMPTZ
    , scope TEXT
    , id_token TEXT
    , hashed_password TEXT
    , created_at TIMESTAMPTZ NOT NULL DEFAULT now()
    , updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
    , CONSTRAINT fk_customer_accounts_customer_id FOREIGN KEY (
        customer_id
    ) REFERENCES customers (
        id
    ) ON DELETE CASCADE
    , CONSTRAINT check_customer_accounts_provider_id CHECK (provider_id IN (
        'credential'
        , 'google'
    ))
);

CREATE TABLE IF NOT EXISTS sessions (
    id UUID NOT NULL PRIMARY KEY DEFAULT uuidv7()
    , identity_id UUID NOT NULL
    , token TEXT NOT NULL
    , service TEXT NOT NULL
    , expires_at TIMESTAMPTZ NOT NULL
    , ip_address TEXT
    , user_agent TEXT
    , created_at TIMESTAMPTZ NOT NULL DEFAULT now()
    , updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
    , CONSTRAINT fk_sessions_identity_id FOREIGN KEY (
        identity_id
    ) REFERENCES identities (id) ON DELETE CASCADE
    , CONSTRAINT check_sessions_service CHECK (service IN (
        'admin_panel'
        , 'merchant_panel'
        , 'buyer_platform'
    ))
);

CREATE TABLE IF NOT EXISTS organizations (
    id UUID NOT NULL PRIMARY KEY DEFAULT uuidv7()
    , parent_id UUID
    , name TEXT NOT NULL
    , slug TEXT NOT NULL
    , status TEXT NOT NULL
    , type TEXT NOT NULL
    , logo TEXT
    , metadata JSONB
    , created_at TIMESTAMPTZ NOT NULL DEFAULT now()
    , CONSTRAINT fk_organizations_parent_id FOREIGN KEY (
        parent_id
    ) REFERENCES organizations (id) ON DELETE CASCADE
    , CONSTRAINT check_organization_type CHECK (type IN (
        'platform'
        , 'merchant'
        , 'individual'
        , 'company'
    ))
    , CONSTRAINT check_organization_status CHECK (status IN (
        'active'
        , 'pending'
        , 'suspended'
    ))
);

CREATE TABLE IF NOT EXISTS members (
    id UUID NOT NULL PRIMARY KEY DEFAULT uuidv7()
    , identity_id UUID NOT NULL
    , organization_id UUID NOT NULL
    , created_at TIMESTAMPTZ NOT NULL DEFAULT now()
    , CONSTRAINT fk_members_identity_id FOREIGN KEY (
        identity_id
    ) REFERENCES identities (id) ON DELETE CASCADE
    , CONSTRAINT fk_members_organization_id FOREIGN KEY (
        organization_id
    ) REFERENCES organizations (id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS roles (
    id SMALLSERIAL PRIMARY KEY
    , role_name TEXT NOT NULL
    , organization_id UUID
    , organization_type TEXT NOT NULL
    , slug TEXT NOT NULL
    , is_system BOOLEAN NOT NULL
    , created_at TIMESTAMPTZ NOT NULL DEFAULT now()
    , updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
    , CONSTRAINT fk_roles_organization_id FOREIGN KEY (
        organization_id
    ) REFERENCES organizations (id) ON DELETE CASCADE
    , CONSTRAINT check_roles_organization_type CHECK (organization_type IN (
        'platform'
        , 'merchant'
        , 'individual'
        , 'company'
    ))
);

CREATE TABLE IF NOT EXISTS member_roles (
    member_id UUID NOT NULL
    , role_id SMALLINT NOT NULL
    , assigned_by UUID
    , created_at TIMESTAMPTZ NOT NULL DEFAULT now()
    , CONSTRAINT member_roles_pkey PRIMARY KEY (member_id, role_id)
    , CONSTRAINT fk_member_roles_member_id FOREIGN KEY (
        member_id
    ) REFERENCES members (id) ON DELETE CASCADE
    , CONSTRAINT fk_member_roles_role_id FOREIGN KEY (
        role_id
    ) REFERENCES roles (id) ON DELETE CASCADE
    , CONSTRAINT fk_member_roles_assigned_by FOREIGN KEY (
        assigned_by
    ) REFERENCES members (id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS addresses (
    id UUID NOT NULL PRIMARY KEY DEFAULT uuidv7()
    , organization_id UUID NOT NULL
    , type TEXT NOT NULL
    , label TEXT NOT NULL
    , line1 TEXT NOT NULL
    , line2 TEXT
    , postal_code TEXT NOT NULL
    , city TEXT NOT NULL
    , state TEXT NOT NULL
    , country TEXT NOT NULL
    , is_default_shipping BOOL NOT NULL DEFAULT false
    , is_default_billing BOOL NOT NULL DEFAULT false
    , created_at TIMESTAMPTZ NOT NULL DEFAULT now()
    , updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
    , CONSTRAINT fk_addresses_organization_id FOREIGN KEY (organization_id)
    REFERENCES organizations (id) ON DELETE CASCADE
    , CONSTRAINT check_addresses_type CHECK (type IN (
        'shipping'
        , 'billing'
        , 'warehouse'
        , 'general'
    ))
    , CONSTRAINT check_address_label_length
    CHECK (length(label) <= 100)
);

CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_users_updated_at
BEFORE UPDATE ON users
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_user_accounts_updated_at
BEFORE UPDATE ON user_accounts
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_customers_updated_at
BEFORE UPDATE ON customers
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_customer_accounts_updated_at
BEFORE UPDATE ON customer_accounts
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_roles_updated_at
BEFORE UPDATE ON roles
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_addresses_updated_at
BEFORE UPDATE ON addresses
FOR EACH ROW EXECUTE FUNCTION set_updated_at();
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS trg_addresses_updated_at ON addresses;
DROP TRIGGER IF EXISTS trg_roles_updated_at ON roles;
DROP TRIGGER IF EXISTS trg_customer_accounts_updated_at ON customer_accounts;
DROP TRIGGER IF EXISTS trg_customers_updated_at ON customers;
DROP TRIGGER IF EXISTS trg_user_accounts_updated_at ON user_accounts;
DROP TRIGGER IF EXISTS trg_users_updated_at ON users;
DROP FUNCTION IF EXISTS set_updated_at() CASCADE;
DROP TABLE addresses;
DROP TABLE member_roles;
DROP TABLE roles;
DROP TABLE members;
DROP TABLE organizations;
DROP TABLE sessions;
DROP TABLE customer_accounts;
DROP TABLE customers;
DROP TABLE user_accounts;
DROP TABLE users;
DROP TABLE identities;
-- +goose StatementEnd
