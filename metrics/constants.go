package metrics

const (
	DefaultPrometheusPath = "/metrics"

	TemporalProxyPrefix = "temporal_cloud_proxy_"

	// Encryption metrics
	EncryptLatency  = TemporalProxyPrefix + "encrypt_latency"
	EncryptRequests = TemporalProxyPrefix + "encrypt_requests"
	EncryptErrors   = TemporalProxyPrefix + "encrypt_errors"
	EncryptSuccess  = TemporalProxyPrefix + "encrypt_success"

	// Decryption metrics
	DecryptLatency  = TemporalProxyPrefix + "decrypt_latency"
	DecryptRequests = TemporalProxyPrefix + "decrypt_requests"
	DecryptErrors   = TemporalProxyPrefix + "decrypt_errors"
	DecryptSuccess  = TemporalProxyPrefix + "decrypt_success"

	// Materials manager get metrics
	MaterialsManagerGetLatency  = TemporalProxyPrefix + "materials_manager_get_latency"
	MaterialsManagerGetRequests = TemporalProxyPrefix + "materials_manager_get_requests"
	MaterialsManagerGetErrors   = TemporalProxyPrefix + "materials_manager_get_errors"
	MaterialsManagerGetSuccess  = TemporalProxyPrefix + "materials_manager_get_success"

	// Materials manager decrypt metrics
	MaterialsManagerDecryptLatency  = TemporalProxyPrefix + "materials_manager_decrypt_latency"
	MaterialsManagerDecryptRequests = TemporalProxyPrefix + "materials_manager_decrypt_requests"
	MaterialsManagerDecryptErrors   = TemporalProxyPrefix + "materials_manager_decrypt_errors"
	MaterialsManagerDecryptSuccess  = TemporalProxyPrefix + "materials_manager_decrypt_success"
)
