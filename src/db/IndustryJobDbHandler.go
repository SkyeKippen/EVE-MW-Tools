package db

import (
	"MeadowWorksTools/src/esi"
	"context"

	"github.com/jackc/pgx/v5"
)

// SaveIndustryJobs stores the owner's jobs, updating ones already saved as
// they progress.
func SaveIndustryJobs(ctx context.Context, ownerID int64, ownerType string, jobs []esi.IndustryJob) error {
	batch := &pgx.Batch{}
	for _, job := range jobs {
		batch.Queue(`INSERT INTO lifetime_indy.industry_jobs
			(owner_id, owner_type, job_id, activity_id, status, installer_id, blueprint_id, blueprint_type_id,
			blueprint_location_id, output_location_id, facility_id, location_id, product_type_id, runs,
			licensed_runs, successful_runs, probability, cost, duration, start_date, end_date, pause_date,
			completed_date, completed_character_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24)
			ON CONFLICT (owner_id, job_id) DO UPDATE SET
			status = EXCLUDED.status, successful_runs = EXCLUDED.successful_runs, end_date = EXCLUDED.end_date,
			pause_date = EXCLUDED.pause_date, completed_date = EXCLUDED.completed_date,
			completed_character_id = EXCLUDED.completed_character_id, updated_at = now()`,
			ownerID, ownerType, job.JobId, job.ActivityId, job.Status, job.InstallerId, job.BlueprintId,
			job.BlueprintTypeId, job.BlueprintLocationId, job.OutputLocationId, job.FacilityId, job.LocationId,
			nullIfZero(job.ProductTypeId), job.Runs, nullIfZero(job.LicensedRuns), nullIfZero(job.SuccessfulRuns),
			nullIfZero(job.Probability), nullIfZero(job.Cost), job.Duration, job.StartDate, job.EndDate,
			nullTime(job.PauseDate), nullTime(job.CompletedDate), nullIfZero(job.CompletedCharacterId))
	}
	return sendBatch(ctx, batch)
}
