CREATE TABLE users
(
    id         BIGSERIAL PRIMARY KEY,
    status     TEXT        NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT users_status_check
        CHECK ( status IN ('active','disabled') )
);
CREATE INDEX idx_users_status ON users(status);

CREATE TABLE deposit_addresses
(
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    chain_id BIGINT NOT NULL,
    address TEXT NOT NULL,
    address_lower TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT fk_deposit_addresses_user FOREIGN KEY (user_id) REFERENCES users(id),
    CONSTRAINT deposit_addresses_status_check CHECK ( status IN ('active','disabled')),
    CONSTRAINT deposit_addresses_address_lower_check CHECK ( address_lower = lower(address))
);
CREATE UNIQUE INDEX ue_deposit_addresses_chain_id_address_lower ON deposit_addresses(chain_id,address_lower);
CREATE INDEX idx_deposit_addresses_user_chain ON deposit_addresses(user_id,chain_id);
CREATE INDEX idx_deposit_addresses_status ON deposit_addresses(status);

CREATE TABLE deposits
(
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    chain_id BIGINT NOT NULL,
    deposit_address_id BIGINT NOT NULL ,
    tx_hash TEXT NOT NULL ,
    block_number BIGINT NOT NULL ,
    block_hash TEXT NOT NULL ,
    from_address TEXT NOT NULL ,
    to_address TEXT NOT NULL ,
    amount_wei NUMERIC(78,0) NOT NULL ,
    status TEXT NOT NULL DEFAULT 'confirming',
    receipt_status SMALLINT NOT NULL ,
    credited_at TIMESTAMP,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT fk_deposit_user FOREIGN KEY (user_id) REFERENCES users(id),
    CONSTRAINT fk_deposits_deposit_address_id FOREIGN KEY (deposit_address_id) REFERENCES deposit_addresses(id),
    CONSTRAINT deposits_status_check CHECK ( status IN ('confirming','credited','failed','reorged') ),
    CONSTRAINT deposits_receipt_status_check CHECK ( receipt_status IN (0,1) ),
    CONSTRAINT deposits_amount_wei_positive_check CHECK ( amount_wei > 0 ),
    CONSTRAINT deposits_credited_at_check CHECK (
        (status = 'credited' AND credited_at IS NOT NULL ) OR (status <> 'credited' AND credited_at IS NULL)
        )
);
CREATE UNIQUE INDEX ue_deposits_chain_id_tx_hash ON deposits(chain_id,tx_hash);
CREATE INDEX idx_deposits_user_id_created_at ON deposits(user_id,created_at);
CREATE INDEX idx_deposits_chain_id_status_block_number ON deposits(chain_id,status,block_number);
CREATE INDEX idx_deposits_chain_id_block_number ON deposits(chain_id,block_number);
CREATE INDEX idx_deposits_deposit_address_id ON deposits(deposit_address_id);

CREATE TABLE balance_accounts
(
    id BIGSERIAL PRIMARY KEY ,
    user_id BIGINT NOT NULL ,
    chain_id BIGINT NOT NULL ,
    asset_symbol TEXT NOT NULL ,
    available_balance NUMERIC(78,0) NOT NULL ,
    frozen_balance NUMERIC(78,0) NOT NULL ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT fk_balance_accounts_user_id FOREIGN KEY (user_id) REFERENCES users(id),
    CONSTRAINT balance_accounts_available_balance_non_negative_check CHECK ( available_balance >= 0 ),
    CONSTRAINT balance_accounts_frozen_balance_non_negative_check CHECK ( frozen_balance >= 0 ),
    CONSTRAINT balance_accounts_asset_symbol_check CHECK ( asset_symbol IN ('ETH') )
);
CREATE UNIQUE INDEX ue_balance_accounts_user_id_chain_id_asset_symbol ON balance_accounts(user_id,chain_id,asset_symbol);

CREATE TABLE balance_ledgers
(
    id BIGSERIAL PRIMARY KEY ,
    user_id BIGINT NOT NULL ,
    chain_id BIGINT NOT NULL ,
    asset_symbol TEXT NOT NULL ,
    amount_wei NUMERIC(78,0) NOT NULL ,
    direction TEXT NOT NULL ,
    reason TEXT NOT NULL ,
    source_type TEXT NOT NULL ,
    source_id BIGINT NOT NULL ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT fk_balance_ledger_user_id FOREIGN KEY (user_id) REFERENCES users(id),
    CONSTRAINT balance_ledger_asset_symbol_check CHECK ( asset_symbol IN ('ETH') ),
    CONSTRAINT balance_ledger_amount_wei_check CHECK ( amount_wei > 0 ),
    CONSTRAINT balance_ledger_direction_check CHECK ( direction IN ('credit','debit') ),
    CONSTRAINT balance_ledger_source_type_check CHECK ( source_type IN ('deposit') ),
    CONSTRAINT balance_ledger_reason_check CHECK ( reason IN ('deposit_credit') )
);
CREATE UNIQUE INDEX ue_balance_ledger_source_type_source_id ON balance_ledgers(source_type,source_id);
CREATE INDEX idx_balance_ledger_asset_symbol ON balance_ledgers(asset_symbol);
CREATE INDEX idx_balance_ledger_created_at ON balance_ledgers(created_at);

CREATE TABLE wallet_scanner_cursors
(
    id BIGSERIAL PRIMARY KEY ,
    chain_id BIGINT NOT NULL ,
    scanner_name TEXT NOT NULL ,
    last_scanned_block_number BIGINT NOT NULL ,
    last_scanned_block_hash TEXT NOT NULL ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT wallet_scanner_cursors_chain_id_positive_check CHECK ( chain_id > 0 ),
    CONSTRAINT wallet_scanner_cursors_last_sync_block_number_check CHECK ( last_scanned_block_number >= 0 ),
    CONSTRAINT wallet_scanner_cursors_scanner_name_not_empty_check CHECK ( length(trim(scanner_name)) > 0 ),
    CONSTRAINT wallet_scanner_cursors_last_sync_block_hash_not_empty_check CHECK ( length(trim(last_scanned_block_hash)) > 0 )
);
CREATE UNIQUE INDEX ue_wallet_scanner_cursors_chain_id_scanner_name ON wallet_scanner_cursors(chain_id,scanner_name);