/**
 * Pure helpers for reconciling Home dashboard counts so categorised
 * buckets sum to the cluster total. Without these, ring segments
 * undercount and users see e.g. "Deployments 100" with breakdown
 * "88 available, 9 unavailable" — leaving 3 unaccounted for and
 * looking like a bug. (SKY-827 bugs 14 + 18)
 *
 * Backend reports total/healthy/warning/error/etc. independently,
 * not as a strict partition; the frontend has to derive the
 * remainder so the ring sums to the label.
 */

/**
 * Pods that are neither healthy nor explicitly warning/error
 * (typically Pending, ContainerCreating, or otherwise transitioning).
 *
 *   computePodTransientCount({ total: 235, healthy: 200, warning: 10, error: 20 }) === 5
 *
 * Returns 0 when the categorised buckets already meet or exceed the
 * total — defensive against backend overcounting (which we don't
 * want to surface as a negative segment).
 */
export function computePodTransientCount(args: {
  total: number
  healthy: number
  warning: number
  error: number
}): number {
  const categorised = args.healthy + args.warning + args.error
  return Math.max(0, args.total - categorised)
}

/**
 * Deployments mid-rollout — backend reports total/available/
 * unavailable independently and Deployments in `progressing` state
 * don't fall into either bucket.
 *
 *   computeDeploymentsProgressing({ total: 100, available: 88, unavailable: 9 }) === 3
 *
 * Returns 0 when available + unavailable already meet or exceed the
 * total.
 */
export function computeDeploymentsProgressing(args: {
  total: number
  available: number
  unavailable: number
}): number {
  return Math.max(0, args.total - args.available - args.unavailable)
}
