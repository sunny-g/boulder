package sql

import (
	"database/sql"
	"errors"
	"fmt"
	"github.com/letsencrypt/boulder/Godeps/_workspace/src/github.com/cloudflare/cfssl/certdb"
	"time"

	"github.com/kisielk/sqlstruct"
	cferr "github.com/letsencrypt/boulder/Godeps/_workspace/src/github.com/cloudflare/cfssl/errors"
)

const (
	insertSQL = `
INSERT INTO certificates (serial, ca_label, status, reason, expiry, revoked_at, pem)
	VALUES ($1, $2, $3, $4, $5, $6, $7);`

	selectSQL = `
SELECT %s FROM certificates
	WHERE (serial = $1);`

	selectAllSQL = `
SELECT %s FROM certificates;`

	selectAllUnexpiredSQL = `
SELECT %s FROM certificates
WHERE CURRENT_TIMESTAMP < expiry;`

	updateRevokeSQL = `
UPDATE certificates
	SET status='revoked', revoked_at=CURRENT_TIMESTAMP, reason=$1
	WHERE (serial = $2);`

	insertOCSPSQL = `
INSERT INTO ocsp_responses (serial, body, expiry)
    VALUES ($1, $2, $3);`

	updateOCSPSQL = `
UPDATE ocsp_responses
    SET expiry=$3, body=$2
	WHERE (serial = $1);`

	selectAllUnexpiredOCSPSQL = `
SELECT %s FROM ocsp_responses
WHERE CURRENT_TIMESTAMP < expiry;`

	selectOCSPSQL = `
SELECT %s FROM ocsp_responses
    WHERE (serial = $1);`
)

// Accessor implements certdb.Accessor interface.
type Accessor struct {
	db *sql.DB
}

func wrapSQLError(err error) error {
	if err != nil {
		return cferr.Wrap(cferr.CertStoreError, cferr.Unknown, err)
	}
	return nil
}

func (d *Accessor) checkDB() error {
	if d.db == nil {
		return cferr.Wrap(cferr.CertStoreError, cferr.Unknown,
			errors.New("unknown db object, please check SetDB method"))
	}
	return nil
}

// NewAccessor returns a new Accessor.
func NewAccessor(db *sql.DB) *Accessor {
	return &Accessor{db: db}
}

// SetDB changes the underlying sql.DB object Accessor is manipulating.
func (d *Accessor) SetDB(db *sql.DB) {
	d.db = db
	return
}

// InsertCertificate puts a certdb.CertificateRecord into db.
func (d *Accessor) InsertCertificate(cr *certdb.CertificateRecord) error {
	err := d.checkDB()
	if err != nil {
		return err
	}

	res, err := d.db.Exec(
		insertSQL,
		cr.Serial,
		cr.CALabel,
		cr.Status,
		cr.Reason,
		cr.Expiry.UTC(),
		cr.RevokedAt,
		cr.PEM,
	)
	if err != nil {
		return wrapSQLError(err)
	}

	numRowsAffected, _ := res.RowsAffected()

	if numRowsAffected == 0 {
		return cferr.Wrap(cferr.CertStoreError, cferr.InsertionFailed, fmt.Errorf("failed to insert the certificate record"))
	}

	if numRowsAffected != 1 {
		return wrapSQLError(fmt.Errorf("%d rows are affected, should be 1 row", numRowsAffected))
	}

	return nil
}

// GetCertificate gets a certdb.CertificateRecord indexed by serial.
func (d *Accessor) GetCertificate(serial string) (*certdb.CertificateRecord, error) {
	err := d.checkDB()
	if err != nil {
		return nil, err
	}

	cr := new(certdb.CertificateRecord)
	rows, err := d.db.Query(fmt.Sprintf(selectSQL, sqlstruct.Columns(*cr)), serial)
	if err != nil {
		return nil, wrapSQLError(err)
	}
	defer rows.Close()

	if rows.Next() {
		return cr, wrapSQLError(sqlstruct.Scan(cr, rows))
	}
	return nil, nil
}

// GetUnexpiredCertificates gets all unexpired certificate from db.
func (d *Accessor) GetUnexpiredCertificates() (crs []*certdb.CertificateRecord, err error) {
	err = d.checkDB()
	if err != nil {
		return nil, err
	}

	cr := new(certdb.CertificateRecord)
	rows, err := d.db.Query(fmt.Sprintf(selectAllUnexpiredSQL, sqlstruct.Columns(*cr)))
	if err != nil {
		return nil, wrapSQLError(err)
	}
	defer rows.Close()

	for rows.Next() {
		err = sqlstruct.Scan(cr, rows)
		if err != nil {
			return nil, wrapSQLError(err)
		}
		crs = append(crs, cr)
	}

	return crs, nil
}

// RevokeCertificate updates a certificate with a given serial number and marks it revoked.
func (d *Accessor) RevokeCertificate(serial string, reasonCode int) error {
	err := d.checkDB()
	if err != nil {
		return err
	}

	result, err := d.db.Exec(updateRevokeSQL, reasonCode, serial)

	if err != nil {
		return wrapSQLError(err)
	}

	numRowsAffected, _ := result.RowsAffected()

	if numRowsAffected == 0 {
		return cferr.Wrap(cferr.CertStoreError, cferr.RecordNotFound, fmt.Errorf("failed to revoke the certificate: certificate not found"))
	}

	if numRowsAffected != 1 {
		return wrapSQLError(fmt.Errorf("%d rows are affected, should be 1 row", numRowsAffected))
	}

	return nil
}

// InsertOCSP puts a new certdb.OCSPRecord into the db.
func (d *Accessor) InsertOCSP(rr *certdb.OCSPRecord) error {
	err := d.checkDB()
	if err != nil {
		return err
	}

	res, err := d.db.Exec(
		insertOCSPSQL,
		rr.Serial,
		rr.Body,
		rr.Expiry.UTC(),
	)
	if err != nil {
		return wrapSQLError(err)
	}

	numRowsAffected, _ := res.RowsAffected()

	if numRowsAffected == 0 {
		return cferr.Wrap(cferr.CertStoreError, cferr.InsertionFailed, fmt.Errorf("failed to insert the OCSP record"))
	}

	if numRowsAffected != 1 {
		return wrapSQLError(fmt.Errorf("%d rows are affected, should be 1 row", numRowsAffected))
	}

	return nil
}

// GetOCSP retrieves a certdb.OCSPRecord from db by serial.
func (d *Accessor) GetOCSP(serial string) (rr *certdb.OCSPRecord, err error) {
	err = d.checkDB()
	if err != nil {
		return nil, err
	}

	rr = new(certdb.OCSPRecord)
	rows, err := d.db.Query(fmt.Sprintf(selectOCSPSQL, sqlstruct.Columns(*rr)), serial)
	if err != nil {
		return nil, wrapSQLError(err)
	}
	defer rows.Close()

	if rows.Next() {
		return rr, sqlstruct.Scan(rr, rows)
	}
	return nil, nil
}

// GetUnexpiredOCSPs retrieves all unexpired certdb.OCSPRecord from db.
func (d *Accessor) GetUnexpiredOCSPs() (rrs []*certdb.OCSPRecord, err error) {
	err = d.checkDB()
	if err != nil {
		return nil, err
	}

	rr := new(certdb.OCSPRecord)
	rows, err := d.db.Query(fmt.Sprintf(selectAllUnexpiredOCSPSQL, sqlstruct.Columns(*rr)))
	if err != nil {
		return nil, wrapSQLError(err)
	}
	defer rows.Close()

	for rows.Next() {
		err = sqlstruct.Scan(rr, rows)
		if err != nil {
			return nil, wrapSQLError(err)
		}
		rrs = append(rrs, rr)
	}

	return rrs, nil
}

// UpdateOCSP updates a ocsp response record with a given serial number.
func (d *Accessor) UpdateOCSP(serial, body string, expiry time.Time) error {
	err := d.checkDB()
	if err != nil {
		return err
	}

	result, err := d.db.Exec(updateOCSPSQL, serial, body, expiry)

	if err != nil {
		return wrapSQLError(err)
	}

	numRowsAffected, err := result.RowsAffected()

	if numRowsAffected == 0 {
		return cferr.Wrap(cferr.CertStoreError, cferr.RecordNotFound, fmt.Errorf("failed to update the OCSP record"))
	}

	if numRowsAffected != 1 {
		return wrapSQLError(fmt.Errorf("%d rows are affected, should be 1 row", numRowsAffected))
	}
	return err
}

// UpsertOCSP update a ocsp response record with a given serial number,
// or insert the record if it doesn't yet exist in the db
// Implementation note:
// We didn't implement 'upsert' with SQL statement and we lost race condition
// prevention provided by underlying DBMS.
// Reasoning:
// 1. it's diffcult to support multiple DBMS backends in the same time, the
// SQL syntax differs from one to another.
// 2. we don't need a strict simultaneous consistency between OCSP and certificate
// status. It's OK that a OCSP response still shows 'good' while the
// corresponding certificate is being revoked seconds ago, as long as the OCSP
// response catches up to be eventually consistent (within hours to days).
// Write race condition between OCSP writers on OCSP table is not a problem,
// since we don't have write race condition on Certificate table and OCSP
// writers should periodically use Certificate table to update OCSP table
// to catch up.
func (d *Accessor) UpsertOCSP(serial, body string, expiry time.Time) error {
	err := d.checkDB()
	if err != nil {
		return err
	}

	result, err := d.db.Exec(updateOCSPSQL, serial, body, expiry)

	if err != nil {
		return wrapSQLError(err)
	}

	numRowsAffected, err := result.RowsAffected()

	if numRowsAffected == 0 {
		return d.InsertOCSP(&certdb.OCSPRecord{Serial: serial, Body: body, Expiry: expiry})
	}

	if numRowsAffected != 1 {
		return wrapSQLError(fmt.Errorf("%d rows are affected, should be 1 row", numRowsAffected))
	}
	return err
}
