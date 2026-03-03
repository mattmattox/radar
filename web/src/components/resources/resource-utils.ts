// Re-export all resource utilities from the shared @skyhook/k8s-ui package.
export * from '@skyhook/k8s-ui/components/resources/resource-utils'

// Backward compatibility: re-export formatBytes (previously re-exported here)
export { formatBytes } from '@skyhook/k8s-ui/utils/format'
