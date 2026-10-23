// Package legal stores the Dokumen Legal data: parties (freelancers,
// subcontractors, clients) and the documents generated for them.
package legal

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/kana-consultant/kantor/backend/internal/model"
	repository "github.com/kana-consultant/kantor/backend/internal/repository"
)

var (
	ErrPartyNotFound = errors.New("legal party not found")
	// ErrPartyInUse: the party still has documents (FK RESTRICT).
	ErrPartyInUse = errors.New("legal party has documents")
)

const partyColumns = `
	p.id::text, p.party_type, p.name, p.address, p.jurisdiction,
	p.id_number_encrypted, p.tax_id_encrypted,
	p.representative_name, p.representative_title, p.contact_name, p.email, p.phone,
	p.bank_name, p.bank_account_encrypted, p.bank_account_holder, p.bank_swift,
	p.notes, p.created_by::text, p.created_at, p.updated_at`

func scanParty(row pgx.Row, item *model.LegalParty, extra ...any) error {
	targets := []any{
		&item.ID, &item.PartyType, &item.Name, &item.Address, &item.Jurisdiction,
		&item.IDNumberEncrypted, &item.TaxIDEncrypted,
		&item.RepresentativeName, &item.RepresentativeTitle, &item.ContactName, &item.Email, &item.Phone,
		&item.BankName, &item.BankAccountEncrypted, &item.BankAccountHolder, &item.BankSwift,
		&item.Notes, &item.CreatedBy, &item.CreatedAt, &item.UpdatedAt,
	}
	return row.Scan(append(targets, extra...)...)
}

type PartiesRepository struct {
	db repository.DBTX
}

func NewPartiesRepository(db repository.DBTX) *PartiesRepository {
	return &PartiesRepository{db: db}
}

// PartyParams are the columns a create or edit writes. The encrypted values
// arrive already encrypted; on an edit a nil pointer keeps the stored value
// and an empty string clears it.
type PartyParams struct {
	PartyType            string
	Name                 string
	Address              string
	Jurisdiction         string
	IDNumberEncrypted    *string
	TaxIDEncrypted       *string
	RepresentativeName   string
	RepresentativeTitle  string
	ContactName          string
	Email                string
	Phone                string
	BankName             string
	BankAccountEncrypted *string
	BankAccountHolder    string
	BankSwift            string
	Notes                string
}

// PartyListItem is a party with the number of documents made for it.
type PartyListItem struct {
	Party     model.LegalParty
	Documents int
}

func (r *PartiesRepository) List(ctx context.Context, search string, partyType string) ([]PartyListItem, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	search = strings.TrimSpace(search)
	rows, err := repository.DB(ctx, r.db).Query(ctx, `
		SELECT `+partyColumns+`,
			(SELECT COUNT(*) FROM legal_documents d WHERE d.party_id = p.id)::int
		FROM legal_parties p
		WHERE ($1 = '' OR p.name ILIKE '%' || $1 || '%' OR p.email ILIKE '%' || $1 || '%')
		  AND ($2 = '' OR p.party_type = $2)
		ORDER BY lower(p.name) ASC, p.created_at ASC
		LIMIT 500
	`, search, strings.TrimSpace(partyType))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]PartyListItem, 0)
	for rows.Next() {
		var item PartyListItem
		if err := scanParty(rows, &item.Party, &item.Documents); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *PartiesRepository) Get(ctx context.Context, id string) (model.LegalParty, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var item model.LegalParty
	err := scanParty(repository.DB(ctx, r.db).QueryRow(ctx, `
		SELECT `+partyColumns+` FROM legal_parties p WHERE p.id = $1::uuid
	`, id), &item)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.LegalParty{}, ErrPartyNotFound
	}
	return item, err
}

func (r *PartiesRepository) Create(ctx context.Context, params PartyParams, createdBy string) (model.LegalParty, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var item model.LegalParty
	err := scanParty(repository.DB(ctx, r.db).QueryRow(ctx, `
		WITH inserted AS (
			INSERT INTO legal_parties (
				party_type, name, address, jurisdiction, id_number_encrypted, tax_id_encrypted,
				representative_name, representative_title, contact_name, email, phone,
				bank_name, bank_account_encrypted, bank_account_holder, bank_swift, notes, created_by
			)
			VALUES ($1, $2, $3, $4, NULLIF($5, ''), NULLIF($6, ''), $7, $8, $9, $10, $11, $12, NULLIF($13, ''), $14, $15, $16, NULLIF($17, '')::uuid)
			RETURNING *
		)
		SELECT `+partyColumns+` FROM inserted p
	`, params.PartyType, params.Name, params.Address, params.Jurisdiction,
		deref(params.IDNumberEncrypted), deref(params.TaxIDEncrypted),
		params.RepresentativeName, params.RepresentativeTitle, params.ContactName, params.Email, params.Phone,
		params.BankName, deref(params.BankAccountEncrypted), params.BankAccountHolder, params.BankSwift, params.Notes, createdBy,
	), &item)
	return item, err
}

func (r *PartiesRepository) Update(ctx context.Context, id string, params PartyParams) (model.LegalParty, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var item model.LegalParty
	err := scanParty(repository.DB(ctx, r.db).QueryRow(ctx, `
		WITH updated AS (
			UPDATE legal_parties SET
				party_type = $2, name = $3, address = $4, jurisdiction = $5,
				id_number_encrypted = CASE WHEN $6::boolean THEN NULLIF($7, '') ELSE id_number_encrypted END,
				tax_id_encrypted = CASE WHEN $8::boolean THEN NULLIF($9, '') ELSE tax_id_encrypted END,
				representative_name = $10, representative_title = $11, contact_name = $12, email = $13, phone = $14,
				bank_name = $15,
				bank_account_encrypted = CASE WHEN $16::boolean THEN NULLIF($17, '') ELSE bank_account_encrypted END,
				bank_account_holder = $18, bank_swift = $19, notes = $20,
				updated_at = NOW()
			WHERE id = $1::uuid
			RETURNING *
		)
		SELECT `+partyColumns+` FROM updated p
	`, id, params.PartyType, params.Name, params.Address, params.Jurisdiction,
		params.IDNumberEncrypted != nil, deref(params.IDNumberEncrypted),
		params.TaxIDEncrypted != nil, deref(params.TaxIDEncrypted),
		params.RepresentativeName, params.RepresentativeTitle, params.ContactName, params.Email, params.Phone,
		params.BankName,
		params.BankAccountEncrypted != nil, deref(params.BankAccountEncrypted),
		params.BankAccountHolder, params.BankSwift, params.Notes,
	), &item)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.LegalParty{}, ErrPartyNotFound
	}
	return item, err
}

func (r *PartiesRepository) Delete(ctx context.Context, id string) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	tag, err := repository.DB(ctx, r.db).Exec(ctx, `DELETE FROM legal_parties WHERE id = $1::uuid`, id)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" {
		return ErrPartyInUse
	}
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrPartyNotFound
	}
	return nil
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
