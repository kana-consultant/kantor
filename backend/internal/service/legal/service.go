package legal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/kana-consultant/kantor/backend/internal/docgen"
	legaldto "github.com/kana-consultant/kantor/backend/internal/dto/legal"
	"github.com/kana-consultant/kantor/backend/internal/mail"
	"github.com/kana-consultant/kantor/backend/internal/model"
	authrepo "github.com/kana-consultant/kantor/backend/internal/repository/auth"
	legalrepo "github.com/kana-consultant/kantor/backend/internal/repository/legal"
	"github.com/kana-consultant/kantor/backend/internal/security"
	hrisservice "github.com/kana-consultant/kantor/backend/internal/service/hris"
	"github.com/kana-consultant/kantor/backend/internal/tenant"
)

// DocumentKind is the document worker kind and the storage directory.
const DocumentKind = "legal"

const (
	partMain   = "doc"
	partSigned = "signed"

	contentTypePDF  = "application/pdf"
	contentTypeDocx = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"

	// MaxSignedCopyBytes caps an uploaded signed copy.
	MaxSignedCopyBytes = 15 << 20
)

var (
	ErrDocumentNotFound    = errors.New("dokumen tidak ditemukan")
	ErrPartyNotFound       = errors.New("pihak tidak ditemukan")
	ErrPartyInUse          = errors.New("pihak ini masih dipakai di dokumen, jadi tidak bisa dihapus")
	ErrStateChanged        = errors.New("dokumen sudah berubah atau sedang dikirim; muat ulang lalu coba lagi")
	ErrNotEditable         = errors.New("dokumen yang sudah ditandatangani atau dibatalkan tidak bisa diubah")
	ErrNotGeneratable      = errors.New("hanya dokumen draf atau yang sudah digenerate yang bisa digenerate ulang")
	ErrNotDeletable        = errors.New("hanya draf yang belum pernah digenerate yang bisa dihapus; batalkan dokumen lainnya")
	ErrPartyTypeNotAllowed = errors.New("jenis pihak ini tidak bisa dipakai untuk jenis dokumen tersebut")
	ErrRelatedInvalid      = errors.New("dokumen acuan tidak valid untuk jenis dokumen ini")
	ErrUnknownType         = errors.New("jenis dokumen tidak dikenal")
	ErrPDFNotReady         = errors.New("PDF dokumen belum siap; tunggu sebentar lalu coba lagi")
	ErrNotSendable         = errors.New("generate dokumen dulu sebelum dikirim")
	ErrSignedCopyInvalid   = errors.New("salinan bertanda tangan harus berupa file PDF (maks. 15 MB)")
	ErrNoSignedCopy        = errors.New("belum ada salinan bertanda tangan")
	ErrSignedDateInvalid   = errors.New("tanggal tanda tangan tidak valid")
)

// FieldsError reports invalid or missing form values, keyed by field.
type FieldsError struct {
	Message string
	Fields  map[string]string
}

func (e *FieldsError) Error() string { return e.Message }

// CompanyIncompleteError: the company profile lacks what every document
// prints.
type CompanyIncompleteError struct {
	Missing []string
}

func (e *CompanyIncompleteError) Error() string {
	return "Profil Perusahaan belum lengkap (" + strings.Join(e.Missing, ", ") + "). Lengkapi di Admin > Settings > Profil Perusahaan."
}

type companyProfile interface {
	Profile(ctx context.Context) (authrepo.CompanyProfileRecord, error)
}

type sequenceRepository interface {
	Next(ctx context.Context, docType string, periodKey string) (int, error)
}

type deliveryRepository interface {
	ListByReference(ctx context.Context, referenceType string, referenceID string) ([]model.EmailDelivery, error)
	HasInFlight(ctx context.Context, referenceType string, referenceID string) (bool, error)
}

type documentQueue interface {
	Enqueue(job hrisservice.DocumentJob) bool
	PDFAvailable() bool
}

type Service struct {
	parties    *legalrepo.PartiesRepository
	documents  *legalrepo.DocumentsRepository
	sequences  sequenceRepository
	company    companyProfile
	deliveries deliveryRepository
	queue      documentQueue
	store      *hrisservice.DocumentStore
	mailer     *hrisservice.DocumentMailer
	encrypter  *security.Encrypter
	now        func() time.Time
}

func NewService(parties *legalrepo.PartiesRepository, documents *legalrepo.DocumentsRepository, sequences sequenceRepository, company companyProfile, deliveries deliveryRepository, queue documentQueue, store *hrisservice.DocumentStore, mailer *hrisservice.DocumentMailer, encrypter *security.Encrypter) *Service {
	return &Service{
		parties: parties, documents: documents, sequences: sequences, company: company, deliveries: deliveries,
		queue: queue, store: store, mailer: mailer, encrypter: encrypter, now: time.Now,
	}
}

// ---------------------------------------------------------------------------
// Parties

var nonDigits = regexp.MustCompile(`[^0-9A-Za-z]`)

func (s *Service) encryptOptional(value *string) (*string, error) {
	if value == nil {
		return nil, nil
	}
	plain := strings.TrimSpace(*value)
	if plain == "" {
		empty := ""
		return &empty, nil
	}
	cipher, err := s.encrypter.EncryptString(plain)
	if err != nil {
		return nil, err
	}
	return &cipher, nil
}

func (s *Service) decryptOptional(value *string) (string, error) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return "", nil
	}
	return s.encrypter.DecryptString(*value)
}

// maskTail shows only the last four characters.
func maskTail(value string) string {
	clean := strings.TrimSpace(value)
	if clean == "" {
		return ""
	}
	runes := []rune(clean)
	if len(runes) <= 4 {
		return strings.Repeat("•", len(runes))
	}
	return strings.Repeat("•", len(runes)-4) + string(runes[len(runes)-4:])
}

func (s *Service) partyResponse(party model.LegalParty, documents int) legaldto.PartyResponse {
	out := legaldto.PartyResponse{
		ID: party.ID, PartyType: party.PartyType, Name: party.Name, Address: party.Address, Jurisdiction: party.Jurisdiction,
		RepresentativeName: party.RepresentativeName, RepresentativeTitle: party.RepresentativeTitle,
		ContactName: party.ContactName, Email: party.Email, Phone: party.Phone,
		BankName: party.BankName, BankAccountHolder: party.BankAccountHolder, BankSwift: party.BankSwift,
		Notes: party.Notes, Documents: documents, CreatedAt: party.CreatedAt, UpdatedAt: party.UpdatedAt,
	}
	// A value that cannot be decrypted is shown as present but hidden.
	for _, item := range []struct {
		cipher *string
		target *string
	}{{party.IDNumberEncrypted, &out.IDNumberMasked}, {party.TaxIDEncrypted, &out.TaxIDMasked}, {party.BankAccountEncrypted, &out.BankAccountMasked}} {
		if item.cipher == nil || *item.cipher == "" {
			continue
		}
		plain, err := s.decryptOptional(item.cipher)
		if err != nil {
			*item.target = "••••"
			continue
		}
		*item.target = maskTail(plain)
	}
	return out
}

func partyParams(input legaldto.PartyRequest) legalrepo.PartyParams {
	return legalrepo.PartyParams{
		PartyType: input.PartyType, Name: strings.TrimSpace(input.Name), Address: strings.TrimSpace(input.Address),
		Jurisdiction:       strings.TrimSpace(input.Jurisdiction),
		RepresentativeName: strings.TrimSpace(input.RepresentativeName), RepresentativeTitle: strings.TrimSpace(input.RepresentativeTitle),
		ContactName: strings.TrimSpace(input.ContactName), Email: strings.ToLower(strings.TrimSpace(input.Email)),
		Phone: strings.TrimSpace(input.Phone), BankName: strings.TrimSpace(input.BankName),
		BankAccountHolder: strings.TrimSpace(input.BankAccountHolder),
		BankSwift:         strings.ToUpper(strings.TrimSpace(input.BankSwift)), Notes: strings.TrimSpace(input.Notes),
	}
}

func (s *Service) encryptPartySecrets(params *legalrepo.PartyParams, input legaldto.PartyRequest) error {
	var err error
	if params.IDNumberEncrypted, err = s.encryptOptional(input.IDNumber); err != nil {
		return err
	}
	if params.TaxIDEncrypted, err = s.encryptOptional(input.TaxID); err != nil {
		return err
	}
	// Account numbers are stored without spaces or dashes.
	if input.BankAccountNumber != nil {
		cleaned := nonDigits.ReplaceAllString(*input.BankAccountNumber, "")
		input.BankAccountNumber = &cleaned
	}
	params.BankAccountEncrypted, err = s.encryptOptional(input.BankAccountNumber)
	return err
}

func validateParty(input legaldto.PartyRequest) error {
	if strings.TrimSpace(input.Name) == "" {
		return &FieldsError{Message: "Nama pihak wajib diisi", Fields: map[string]string{"name": "wajib diisi"}}
	}
	return nil
}

func (s *Service) ListParties(ctx context.Context, search string, partyType string) ([]legaldto.PartyResponse, error) {
	items, err := s.parties.List(ctx, search, partyType)
	if err != nil {
		return nil, err
	}
	out := make([]legaldto.PartyResponse, 0, len(items))
	for _, item := range items {
		out = append(out, s.partyResponse(item.Party, item.Documents))
	}
	return out, nil
}

func (s *Service) GetParty(ctx context.Context, id string) (legaldto.PartyResponse, error) {
	party, err := s.parties.Get(ctx, id)
	if errors.Is(err, legalrepo.ErrPartyNotFound) {
		return legaldto.PartyResponse{}, ErrPartyNotFound
	}
	if err != nil {
		return legaldto.PartyResponse{}, err
	}
	return s.partyResponse(party, 0), nil
}

func (s *Service) CreateParty(ctx context.Context, actorID string, input legaldto.PartyRequest) (legaldto.PartyResponse, error) {
	if err := validateParty(input); err != nil {
		return legaldto.PartyResponse{}, err
	}
	params := partyParams(input)
	if err := s.encryptPartySecrets(&params, input); err != nil {
		return legaldto.PartyResponse{}, err
	}
	party, err := s.parties.Create(ctx, params, actorID)
	if err != nil {
		return legaldto.PartyResponse{}, err
	}
	return s.partyResponse(party, 0), nil
}

func (s *Service) UpdateParty(ctx context.Context, id string, input legaldto.PartyRequest) (legaldto.PartyResponse, error) {
	if err := validateParty(input); err != nil {
		return legaldto.PartyResponse{}, err
	}
	params := partyParams(input)
	if err := s.encryptPartySecrets(&params, input); err != nil {
		return legaldto.PartyResponse{}, err
	}
	party, err := s.parties.Update(ctx, id, params)
	if errors.Is(err, legalrepo.ErrPartyNotFound) {
		return legaldto.PartyResponse{}, ErrPartyNotFound
	}
	if err != nil {
		return legaldto.PartyResponse{}, err
	}
	return s.partyResponse(party, 0), nil
}

func (s *Service) DeleteParty(ctx context.Context, id string) error {
	err := s.parties.Delete(ctx, id)
	switch {
	case errors.Is(err, legalrepo.ErrPartyNotFound):
		return ErrPartyNotFound
	case errors.Is(err, legalrepo.ErrPartyInUse):
		return ErrPartyInUse
	}
	return err
}

// ---------------------------------------------------------------------------
// Documents

func formatDate(value *time.Time) *string {
	if value == nil {
		return nil
	}
	s := value.Format(dateLayout)
	return &s
}

func listItem(doc model.LegalDocument, partyName string) legaldto.DocumentListItem {
	return legaldto.DocumentListItem{
		ID: doc.ID, DocType: doc.DocType, Status: doc.Status, Title: doc.Title, PartyID: doc.PartyID, PartyName: partyName,
		DocNumber: doc.DocNumber, DocumentDate: formatDate(doc.DocumentDate), RenderStatus: doc.RenderStatus,
		LastSentAt: doc.LastSentAt, SignedAt: formatDate(doc.SignedAt), UpdatedAt: doc.UpdatedAt,
	}
}

func (s *Service) ListDocuments(ctx context.Context, filter legalrepo.DocumentFilter) ([]legaldto.DocumentListItem, error) {
	items, err := s.documents.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	out := make([]legaldto.DocumentListItem, 0, len(items))
	for _, item := range items {
		out = append(out, listItem(item.Document, item.PartyName))
	}
	return out, nil
}

func (s *Service) getDocument(ctx context.Context, id string) (model.LegalDocument, error) {
	doc, err := s.documents.Get(ctx, id)
	if errors.Is(err, legalrepo.ErrDocumentNotFound) {
		return model.LegalDocument{}, ErrDocumentNotFound
	}
	return doc, err
}

func (s *Service) getParty(ctx context.Context, id string) (model.LegalParty, error) {
	party, err := s.parties.Get(ctx, id)
	if errors.Is(err, legalrepo.ErrPartyNotFound) {
		return model.LegalParty{}, ErrPartyNotFound
	}
	return party, err
}

// companyMissing lists what the company profile lacks for documents.
func companyMissing(company authrepo.CompanyProfileRecord, needCity bool) []string {
	var missing []string
	if strings.TrimSpace(company.LegalName) == "" {
		missing = append(missing, "nama perusahaan")
	}
	if strings.TrimSpace(company.Address) == "" {
		missing = append(missing, "alamat")
	}
	if strings.TrimSpace(company.SignerName) == "" || strings.TrimSpace(company.SignerTitle) == "" {
		missing = append(missing, "nama dan jabatan penanda tangan")
	}
	if needCity && strings.TrimSpace(company.City) == "" {
		missing = append(missing, "kota")
	}
	return missing
}

func (s *Service) storedValues(spec *Spec, doc model.LegalDocument) map[string]any {
	values, _ := NormalizeFields(spec, doc.Fields)
	return values
}

func (s *Service) detail(ctx context.Context, doc model.LegalDocument) (legaldto.DocumentDetail, error) {
	spec, ok := SpecFor(doc.DocType)
	if !ok {
		return legaldto.DocumentDetail{}, ErrUnknownType
	}
	party, err := s.getParty(ctx, doc.PartyID)
	if err != nil {
		return legaldto.DocumentDetail{}, err
	}
	company, err := s.company.Profile(ctx)
	if err != nil {
		return legaldto.DocumentDetail{}, err
	}
	values := s.storedValues(spec, doc)
	out := legaldto.DocumentDetail{
		DocumentListItem: listItem(doc, party.Name),
		Fields:           values,
		DocumentCity:     doc.DocumentCity,
		Party:            s.partyResponse(party, 0),
		RenderError:      doc.RenderError,
		PDFReady:         pdfReady(doc),
		HasSignedCopy:    doc.SignedPDFPath != nil,
		GeneratedAt:      doc.GeneratedAt,
		CreatedAt:        doc.CreatedAt,
		Missing:          MissingFields(spec, WithDefaults(spec, values)),
		CompanyMissing:   companyMissing(company, doc.DocumentCity == nil || strings.TrimSpace(*doc.DocumentCity) == ""),
		PDFAvailable:     s.queue != nil && s.queue.PDFAvailable(),
		DefaultRecipient: party.Email,
	}
	if doc.RelatedDocumentID != nil {
		if related, err := s.documents.Get(ctx, *doc.RelatedDocumentID); err == nil {
			out.Related = &legaldto.RelatedDocument{ID: related.ID, DocType: related.DocType, Title: related.Title, DocNumber: related.DocNumber, Status: related.Status}
		}
	}
	if s.deliveries != nil {
		deliveries, err := s.deliveries.ListByReference(ctx, model.LegalDocumentReferenceType, doc.ID)
		if err != nil {
			return legaldto.DocumentDetail{}, err
		}
		for index := range deliveries {
			deliveries[index].Recipient = hrisservice.MaskEmail(deliveries[index].Recipient)
		}
		out.Deliveries = deliveries
		busy, err := s.deliveries.HasInFlight(ctx, model.LegalDocumentReferenceType, doc.ID)
		if err != nil {
			return legaldto.DocumentDetail{}, err
		}
		out.SendInFlight = busy
	}
	if s.mailer != nil {
		ready, err := s.mailer.Ready(ctx)
		if err != nil {
			return legaldto.DocumentDetail{}, err
		}
		out.MailReady = ready
	}
	return out, nil
}

func (s *Service) GetDocument(ctx context.Context, id string) (legaldto.DocumentDetail, error) {
	doc, err := s.getDocument(ctx, id)
	if err != nil {
		return legaldto.DocumentDetail{}, err
	}
	return s.detail(ctx, doc)
}

// documentParams validates a create or edit: the party type must suit the
// document type, a related document must be of an allowed type, and the
// form values must be well formed (not necessarily complete).
func (s *Service) documentParams(ctx context.Context, spec *Spec, input legaldto.DocumentRequest, id string) (legalrepo.DocumentParams, error) {
	party, err := s.getParty(ctx, input.PartyID)
	if err != nil {
		return legalrepo.DocumentParams{}, err
	}
	allowed := false
	for _, partyType := range spec.PartyTypes {
		if partyType == party.PartyType {
			allowed = true
		}
	}
	if !allowed {
		return legalrepo.DocumentParams{}, ErrPartyTypeNotAllowed
	}
	if input.RelatedDocumentID != nil && strings.TrimSpace(*input.RelatedDocumentID) != "" {
		related, err := s.documents.Get(ctx, *input.RelatedDocumentID)
		if err != nil || related.ID == id {
			return legalrepo.DocumentParams{}, ErrRelatedInvalid
		}
		ok := false
		for _, docType := range spec.RelatedTypes {
			if docType == related.DocType {
				ok = true
			}
		}
		if !ok {
			return legalrepo.DocumentParams{}, ErrRelatedInvalid
		}
	} else {
		input.RelatedDocumentID = nil
	}
	values, fieldErrs := NormalizeFields(spec, input.Fields)
	if len(fieldErrs) > 0 {
		return legalrepo.DocumentParams{}, &FieldsError{Message: "Beberapa isian tidak valid", Fields: fieldErrs}
	}
	fields, err := json.Marshal(values)
	if err != nil {
		return legalrepo.DocumentParams{}, err
	}
	params := legalrepo.DocumentParams{
		Title: strings.TrimSpace(input.Title), PartyID: party.ID, RelatedDocumentID: input.RelatedDocumentID, Fields: fields,
	}
	if input.DocumentDate != nil && strings.TrimSpace(*input.DocumentDate) != "" {
		date, err := time.Parse(dateLayout, strings.TrimSpace(*input.DocumentDate))
		if err != nil {
			return legalrepo.DocumentParams{}, &FieldsError{Message: "Tanggal dokumen tidak valid", Fields: map[string]string{"document_date": "format YYYY-MM-DD"}}
		}
		params.DocumentDate = &date
	}
	if input.DocumentCity != nil && strings.TrimSpace(*input.DocumentCity) != "" {
		city := strings.TrimSpace(*input.DocumentCity)
		params.DocumentCity = &city
	}
	return params, nil
}

// CreateDocument starts a draft with the type's defaults filled in.
func (s *Service) CreateDocument(ctx context.Context, actorID string, input legaldto.DocumentRequest) (legaldto.DocumentDetail, error) {
	spec, ok := SpecFor(input.DocType)
	if !ok {
		return legaldto.DocumentDetail{}, ErrUnknownType
	}
	params, err := s.documentParams(ctx, spec, input, "")
	if err != nil {
		return legaldto.DocumentDetail{}, err
	}
	values, _ := NormalizeFields(spec, params.Fields)
	withDefaults, err := json.Marshal(WithDefaults(spec, values))
	if err != nil {
		return legaldto.DocumentDetail{}, err
	}
	params.Fields = withDefaults
	doc, err := s.documents.Create(ctx, spec.Type, params, actorID)
	if errors.Is(err, legalrepo.ErrDocumentPartyMissing) {
		return legaldto.DocumentDetail{}, ErrPartyNotFound
	}
	if err != nil {
		return legaldto.DocumentDetail{}, err
	}
	return s.detail(ctx, doc)
}

// UpdateDocument saves the form. An issued document goes back to draft (its
// number is kept) and has to be generated again.
func (s *Service) UpdateDocument(ctx context.Context, id string, input legaldto.DocumentRequest) (legaldto.DocumentDetail, error) {
	current, err := s.getDocument(ctx, id)
	if err != nil {
		return legaldto.DocumentDetail{}, err
	}
	if current.Status == model.LegalDocStatusSigned || current.Status == model.LegalDocStatusCancelled {
		return legaldto.DocumentDetail{}, ErrNotEditable
	}
	spec, ok := SpecFor(current.DocType)
	if !ok {
		return legaldto.DocumentDetail{}, ErrUnknownType
	}
	params, err := s.documentParams(ctx, spec, input, id)
	if err != nil {
		return legaldto.DocumentDetail{}, err
	}
	doc, oldPDF, err := s.documents.Update(ctx, id, params)
	switch {
	case errors.Is(err, legalrepo.ErrDocumentNotFound):
		return legaldto.DocumentDetail{}, ErrDocumentNotFound
	case errors.Is(err, legalrepo.ErrDocumentStateChanged):
		return legaldto.DocumentDetail{}, ErrStateChanged
	case errors.Is(err, legalrepo.ErrDocumentPartyMissing):
		return legaldto.DocumentDetail{}, ErrPartyNotFound
	case err != nil:
		return legaldto.DocumentDetail{}, err
	}
	s.deleteFile(ctx, oldPDF)
	return s.detail(ctx, doc)
}

func (s *Service) DeleteDocument(ctx context.Context, id string) error {
	if _, err := s.getDocument(ctx, id); err != nil {
		return err
	}
	err := s.documents.DeleteDraft(ctx, id)
	if errors.Is(err, legalrepo.ErrDocumentStateChanged) {
		return ErrNotDeletable
	}
	return err
}

func (s *Service) CancelDocument(ctx context.Context, id string) (legaldto.DocumentDetail, error) {
	if _, err := s.getDocument(ctx, id); err != nil {
		return legaldto.DocumentDetail{}, err
	}
	doc, err := s.documents.Cancel(ctx, id)
	if errors.Is(err, legalrepo.ErrDocumentStateChanged) {
		return legaldto.DocumentDetail{}, ErrStateChanged
	}
	if err != nil {
		return legaldto.DocumentDetail{}, err
	}
	return s.detail(ctx, doc)
}

func (s *Service) partySecrets(party model.LegalParty) (PartySecrets, error) {
	var secrets PartySecrets
	var err error
	if secrets.IDNumber, err = s.decryptOptional(party.IDNumberEncrypted); err != nil {
		return PartySecrets{}, fmt.Errorf("decrypt party identity: %w", err)
	}
	if secrets.TaxID, err = s.decryptOptional(party.TaxIDEncrypted); err != nil {
		return PartySecrets{}, fmt.Errorf("decrypt party tax id: %w", err)
	}
	if secrets.BankAccount, err = s.decryptOptional(party.BankAccountEncrypted); err != nil {
		return PartySecrets{}, fmt.Errorf("decrypt party account: %w", err)
	}
	return secrets, nil
}

// partyMissing lists the party data a document type prints that is empty.
func partyMissing(spec *Spec, party model.LegalParty, secrets PartySecrets) map[string]string {
	missing := map[string]string{}
	if strings.TrimSpace(party.Address) == "" {
		missing["party.address"] = "alamat pihak belum diisi"
	}
	switch spec.Type {
	case model.LegalDocFreelance:
		if secrets.IDNumber == "" {
			missing["party.id_number"] = "NIK mitra belum diisi"
		}
		if strings.TrimSpace(party.BankName) == "" || secrets.BankAccount == "" {
			missing["party.bank"] = "rekening mitra belum diisi"
		}
	case model.LegalDocSubcontract:
		if party.PartyType == model.LegalPartyCompany && strings.TrimSpace(party.RepresentativeName) == "" {
			missing["party.representative_name"] = "nama penanda tangan pihak belum diisi"
		}
		if party.PartyType == model.LegalPartyPerson && secrets.IDNumber == "" {
			missing["party.id_number"] = "nomor identitas pihak belum diisi"
		}
	default:
		if party.PartyType == model.LegalPartyCompany && strings.TrimSpace(party.RepresentativeName) == "" {
			missing["party.representative_name"] = "nama penanda tangan klien belum diisi"
		}
	}
	return missing
}

func (s *Service) enqueue(ctx context.Context, doc model.LegalDocument) {
	if s.queue == nil || doc.RenderStatus != model.DocumentRenderPending {
		return
	}
	info, ok := tenant.FromContext(ctx)
	if !ok {
		return // the sweep picks it up
	}
	s.queue.Enqueue(hrisservice.DocumentJob{Tenant: info, Kind: DocumentKind, ID: doc.ID})
}

func jakartaToday(now time.Time) time.Time {
	location, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		location = time.FixedZone("WIB", 7*60*60)
	}
	local := now.In(location)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
}

// Generate checks the document is complete, assigns its number once (kept
// by later edits), snapshots the template payload (encrypted: it carries the
// party's identity and account numbers) and queues the PDF render.
func (s *Service) Generate(ctx context.Context, actorID string, id string) (legaldto.DocumentDetail, map[string]any, error) {
	doc, err := s.getDocument(ctx, id)
	if err != nil {
		return legaldto.DocumentDetail{}, nil, err
	}
	if doc.Status != model.LegalDocStatusDraft && doc.Status != model.LegalDocStatusGenerated {
		return legaldto.DocumentDetail{}, nil, ErrNotGeneratable
	}
	spec, ok := SpecFor(doc.DocType)
	if !ok {
		return legaldto.DocumentDetail{}, nil, ErrUnknownType
	}
	company, err := s.company.Profile(ctx)
	if err != nil {
		return legaldto.DocumentDetail{}, nil, err
	}

	firstNumber := false
	var generated model.LegalDocument
	err = s.documents.WithTx(ctx, func(txCtx context.Context) error {
		locked, err := s.documents.GetForUpdate(txCtx, id)
		if err != nil {
			return err
		}
		if locked.Status != model.LegalDocStatusDraft && locked.Status != model.LegalDocStatusGenerated {
			return ErrNotGeneratable
		}
		party, err := s.getParty(txCtx, locked.PartyID)
		if err != nil {
			return err
		}
		secrets, err := s.partySecrets(party)
		if err != nil {
			return err
		}
		city := ""
		if locked.DocumentCity != nil {
			city = strings.TrimSpace(*locked.DocumentCity)
		}
		if city == "" {
			city = strings.TrimSpace(company.City)
		}
		if missing := companyMissing(company, city == ""); len(missing) > 0 {
			return &CompanyIncompleteError{Missing: missing}
		}
		values := WithDefaults(spec, s.storedValues(spec, locked))
		missing := MissingFields(spec, values)
		for key, message := range partyMissing(spec, party, secrets) {
			missing[key] = message
		}
		if len(missing) > 0 {
			return &FieldsError{Message: "Lengkapi isian yang wajib dulu sebelum generate", Fields: missing}
		}

		documentDate := jakartaToday(s.now())
		if locked.DocumentDate != nil {
			documentDate = *locked.DocumentDate
		}
		firstNumber = locked.SeqNo == nil
		seq, number := 0, ""
		if firstNumber {
			next, err := s.sequences.Next(txCtx, spec.sequence, documentDate.Format("2006-01"))
			if err != nil {
				return fmt.Errorf("next legal document number: %w", err)
			}
			seq, number = next, FormatNumber(spec.Code, next, company.DocCode, documentDate)
		} else {
			seq, number = *locked.SeqNo, *locked.DocNumber
		}
		payload := BuildPayload(spec, values, company, party, secrets, number, documentDate, city)
		raw, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		cipher, err := s.encrypter.EncryptString(string(raw))
		if err != nil {
			return err
		}
		tpl, err := docgen.Load(spec.Template)
		if err != nil {
			return err
		}
		renderStatus := model.DocumentRenderNone
		if s.queue != nil && s.queue.PDFAvailable() {
			renderStatus = model.DocumentRenderPending
		}
		generated, err = s.documents.RecordSnapshot(txCtx, id, legalrepo.SnapshotParams{
			ExpectStatus: locked.Status, SeqNo: seq, DocNumber: number, DocumentDate: documentDate, DocumentCity: city,
			PayloadEncrypted: cipher, TemplateVersion: tpl.Version, GeneratedBy: actorID, RenderStatus: renderStatus,
		})
		return err
	})
	switch {
	case errors.Is(err, legalrepo.ErrDocumentStateChanged):
		return legaldto.DocumentDetail{}, nil, ErrStateChanged
	case errors.Is(err, legalrepo.ErrDocumentNotFound):
		return legaldto.DocumentDetail{}, nil, ErrDocumentNotFound
	case err != nil:
		return legaldto.DocumentDetail{}, nil, err
	}
	s.enqueue(ctx, generated)
	detail, err := s.detail(ctx, generated)
	if err != nil {
		return legaldto.DocumentDetail{}, nil, err
	}
	audit := map[string]any{"doc_type": generated.DocType, "doc_number": generated.DocNumber, "number_assigned": firstNumber, "render_status": generated.RenderStatus}
	return detail, audit, nil
}

func pdfReady(doc model.LegalDocument) bool {
	return doc.RenderStatus == model.DocumentRenderReady && doc.PDFPath != nil && doc.PDFSHA256 != nil
}

// File is a document file to return or attach.
type File struct {
	Data        []byte
	Filename    string
	ContentType string
	Document    model.LegalDocument
}

var filenameUnsafe = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func fileBase(doc model.LegalDocument) string {
	spec, _ := SpecFor(doc.DocType)
	base := doc.DocType
	if spec != nil {
		base = spec.ShortLabel
	}
	if doc.DocNumber != nil {
		base += "_" + strings.ReplaceAll(*doc.DocNumber, "/", "-")
	}
	return strings.Trim(filenameUnsafe.ReplaceAllString(base, "_"), "_")
}

func (s *Service) readStored(ctx context.Context, path string, sha string) ([]byte, error) {
	info, ok := tenant.FromContext(ctx)
	if !ok {
		return nil, errors.New("tenant is missing from context")
	}
	return s.store.Read(info.ID, path, sha)
}

func (s *Service) deleteFile(ctx context.Context, path *string) {
	if path == nil || *path == "" || s.store == nil {
		return
	}
	info, ok := tenant.FromContext(ctx)
	if !ok {
		return
	}
	if err := s.store.Delete(info.ID, *path); err != nil {
		slog.WarnContext(ctx, "legal document: delete superseded file", "error", err)
	}
}

func (s *Service) PDF(ctx context.Context, id string) (File, error) {
	doc, err := s.getDocument(ctx, id)
	if err != nil {
		return File{}, err
	}
	if !pdfReady(doc) {
		return File{}, ErrPDFNotReady
	}
	data, err := s.readStored(ctx, *doc.PDFPath, *doc.PDFSHA256)
	if err != nil {
		return File{}, err
	}
	return File{Data: data, Filename: fileBase(doc) + ".pdf", ContentType: contentTypePDF, Document: doc}, nil
}

func (s *Service) renderDocx(doc model.LegalDocument) ([]byte, error) {
	if doc.PayloadEncrypted == nil {
		return nil, ErrNotSendable
	}
	spec, ok := SpecFor(doc.DocType)
	if !ok {
		return nil, ErrUnknownType
	}
	payload, err := s.decryptPayload(doc)
	if err != nil {
		return nil, err
	}
	tpl, err := docgen.Load(spec.Template)
	if err != nil {
		return nil, err
	}
	return docgen.Render(tpl, payload)
}

func (s *Service) decryptPayload(doc model.LegalDocument) (docgen.Payload, error) {
	plain, err := s.encrypter.DecryptString(*doc.PayloadEncrypted)
	if err != nil {
		return nil, fmt.Errorf("decrypt legal document payload: %w", err)
	}
	var payload docgen.Payload
	decoder := json.NewDecoder(bytes.NewReader([]byte(plain)))
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode legal document payload: %w", err)
	}
	return payload, nil
}

func (s *Service) Docx(ctx context.Context, id string) (File, error) {
	doc, err := s.getDocument(ctx, id)
	if err != nil {
		return File{}, err
	}
	data, err := s.renderDocx(doc)
	if err != nil {
		return File{}, err
	}
	return File{Data: data, Filename: fileBase(doc) + ".docx", ContentType: contentTypeDocx, Document: doc}, nil
}

// MarkSigned records the signing date and, optionally, a scanned signed
// copy (a PDF, stored encrypted).
func (s *Service) MarkSigned(ctx context.Context, id string, signedAt string, scan []byte) (legaldto.DocumentDetail, error) {
	doc, err := s.getDocument(ctx, id)
	if err != nil {
		return legaldto.DocumentDetail{}, err
	}
	date, err := time.Parse(dateLayout, strings.TrimSpace(signedAt))
	if err != nil || date.After(jakartaToday(s.now()).AddDate(0, 0, 1)) {
		return legaldto.DocumentDetail{}, ErrSignedDateInvalid
	}
	if doc.Status != model.LegalDocStatusGenerated && doc.Status != model.LegalDocStatusSent && doc.Status != model.LegalDocStatusSigned {
		return legaldto.DocumentDetail{}, ErrNotSendable
	}
	var scanPath, scanSHA *string
	if len(scan) > 0 {
		if len(scan) > MaxSignedCopyBytes || !bytes.HasPrefix(scan, []byte("%PDF-")) {
			return legaldto.DocumentDetail{}, ErrSignedCopyInvalid
		}
		info, ok := tenant.FromContext(ctx)
		if !ok {
			return legaldto.DocumentDetail{}, errors.New("tenant is missing from context")
		}
		stored, err := s.store.Write(info.ID, DocumentKind, doc.ID, partSigned, scan)
		if err != nil {
			return legaldto.DocumentDetail{}, err
		}
		scanPath, scanSHA = &stored.Path, &stored.SHA256
	}
	updated, replaced, err := s.documents.SetSigned(ctx, id, date, scanPath, scanSHA)
	if errors.Is(err, legalrepo.ErrDocumentStateChanged) {
		s.deleteFile(ctx, scanPath)
		return legaldto.DocumentDetail{}, ErrStateChanged
	}
	if err != nil {
		s.deleteFile(ctx, scanPath)
		return legaldto.DocumentDetail{}, err
	}
	s.deleteFile(ctx, replaced)
	return s.detail(ctx, updated)
}

func (s *Service) SignedCopy(ctx context.Context, id string) (File, error) {
	doc, err := s.getDocument(ctx, id)
	if err != nil {
		return File{}, err
	}
	if doc.SignedPDFPath == nil || doc.SignedPDFSHA256 == nil {
		return File{}, ErrNoSignedCopy
	}
	data, err := s.readStored(ctx, *doc.SignedPDFPath, *doc.SignedPDFSHA256)
	if err != nil {
		return File{}, err
	}
	return File{Data: data, Filename: fileBase(doc) + "_ttd.pdf", ContentType: contentTypePDF, Document: doc}, nil
}

// ---------------------------------------------------------------------------
// Send

// SendResult is the API response plus the audit values (masked recipient).
type SendResult struct {
	Response legaldto.SendResponse
	Audit    map[string]any
}

func (s *Service) attachment(ctx context.Context, doc model.LegalDocument) (mail.Attachment, string, error) {
	if pdfReady(doc) {
		data, err := s.readStored(ctx, *doc.PDFPath, *doc.PDFSHA256)
		if err != nil {
			return mail.Attachment{}, "", err
		}
		return mail.Attachment{Filename: fileBase(doc) + ".pdf", ContentType: contentTypePDF, Data: data}, "pdf", nil
	}
	if (s.queue == nil || !s.queue.PDFAvailable()) && s.mailer.AllowDocxSend() {
		data, err := s.renderDocx(doc)
		if err != nil {
			return mail.Attachment{}, "", err
		}
		return mail.Attachment{Filename: fileBase(doc) + ".docx", ContentType: contentTypeDocx, Data: data}, "docx", nil
	}
	if s.queue == nil || !s.queue.PDFAvailable() {
		return mail.Attachment{}, "", hrisservice.ErrDocumentPDFConverterMissing
	}
	return mail.Attachment{}, "", ErrPDFNotReady
}

func (s *Service) Send(ctx context.Context, actorID string, id string, recipient string) (SendResult, error) {
	doc, err := s.getDocument(ctx, id)
	if err != nil {
		return SendResult{}, err
	}
	if doc.Status != model.LegalDocStatusGenerated && doc.Status != model.LegalDocStatusSent && doc.Status != model.LegalDocStatusSigned {
		return SendResult{}, ErrNotSendable
	}
	ready, err := s.mailer.Ready(ctx)
	if err != nil {
		return SendResult{}, err
	}
	if !ready {
		return SendResult{}, hrisservice.ErrDocumentMailNotReady
	}
	address := strings.ToLower(strings.TrimSpace(recipient))
	if err := mail.ValidateAddress(address); err != nil {
		return SendResult{}, &FieldsError{Message: "Alamat email penerima tidak valid", Fields: map[string]string{"recipient": "alamat email tidak valid"}}
	}
	party, err := s.getParty(ctx, doc.PartyID)
	if err != nil {
		return SendResult{}, err
	}
	attachment, format, err := s.attachment(ctx, doc)
	if err != nil {
		return SendResult{}, err
	}
	spec, _ := SpecFor(doc.DocType)
	number := ""
	if doc.DocNumber != nil {
		number = *doc.DocNumber
	}
	company, err := s.mailer.HRContact(ctx)
	if err != nil {
		return SendResult{}, err
	}
	sender := strings.TrimSpace(company.LegalName)
	paragraphsID := []string{
		fmt.Sprintf("Terlampir %s No. %s untuk Anda pelajari.", spec.Label, number),
		"Mohon periksa dokumen ini. Penandatanganan dilakukan sesuai kesepakatan para pihak.",
	}
	paragraphsEN := []string{
		fmt.Sprintf("Please find attached %s No. %s for your review.", spec.Label, number),
		"Please review the document. Signing follows the parties' agreement.",
	}
	if sender != "" {
		paragraphsID = append(paragraphsID, fmt.Sprintf("Dokumen ini dikirim oleh %s dan hanya ditujukan kepada penerima.", sender))
		paragraphsEN = append(paragraphsEN, fmt.Sprintf("This document is sent by %s and intended for the recipient only.", sender))
	}
	queued, err := s.mailer.Enqueue(ctx, hrisservice.DocumentMail{
		Kind:          model.EmailDeliveryKindLegal,
		ReferenceType: model.LegalDocumentReferenceType,
		ReferenceID:   doc.ID,
		Recipient:     hrisservice.ResolvedRecipient{EmployeeName: party.Name, Address: address, Source: model.EmailRecipientSourceParty},
		Subject:       fmt.Sprintf("%s — %s", spec.Label, number),
		ParagraphsID:  paragraphsID,
		ParagraphsEN:  paragraphsEN,
		Attachments:   []mail.Attachment{attachment},
		RequestedBy:   actorID,
	})
	if err != nil {
		return SendResult{}, err
	}
	// The queued row froze the document: make sure it is still the one
	// whose file is attached.
	current, err := s.getDocument(ctx, id)
	if err != nil || current.UpdatedAt != doc.UpdatedAt {
		s.mailer.CancelQueued(ctx, queued.Delivery.ID, "dokumen berubah sebelum dikirim")
		if err != nil {
			return SendResult{}, err
		}
		return SendResult{}, ErrStateChanged
	}

	delivery, sendErr := s.mailer.DeliverQueued(ctx, queued)
	audit := map[string]any{
		"doc_type": doc.DocType, "doc_number": number, "format": format,
		"recipient": hrisservice.MaskEmail(address), "recipient_is_party_email": strings.EqualFold(address, party.Email),
	}
	result := SendResult{Audit: audit}
	if sendErr != nil {
		classified, ok := mail.AsSendError(sendErr)
		if !ok || delivery.ID == "" {
			return SendResult{}, sendErr
		}
		category, message := string(classified.Category), classified.Message()
		result.Response.ErrorCategory = &category
		result.Response.ErrorMessage = &message
		audit["status"] = model.EmailDeliveryStatusFailed
		audit["error_category"] = category
	} else {
		result.Response.Sent = true
		audit["status"] = model.EmailDeliveryStatusSent
		if updated, err := s.documents.MarkSent(ctx, id, s.now()); err == nil {
			doc = updated
		} else {
			slog.ErrorContext(ctx, "legal document e-mailed but not marked sent", "document_id", id, "error", err)
		}
	}
	audit["delivery_id"] = delivery.ID
	delivery.Recipient = hrisservice.MaskEmail(delivery.Recipient)
	result.Response.Delivery = delivery
	detail, err := s.detail(ctx, doc)
	if err != nil {
		return SendResult{}, err
	}
	result.Response.Document = detail
	return result, nil
}
