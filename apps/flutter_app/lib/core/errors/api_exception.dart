/// Typed error for every failed API call. Server errors carry the envelope
/// `{error:{code,message,details}}`; client-side failures use the synthetic
/// codes in [ApiErrorCodes].
class ApiException implements Exception {
  const ApiException({
    required this.code,
    required this.message,
    this.status = 0,
    this.details,
  });

  final String code;
  final String message;

  /// HTTP status; 0 when no response was received.
  final int status;
  final Map<String, dynamic>? details;

  bool get isUnauthorized =>
      status == 401 || code == ApiErrorCodes.unauthorized;
  bool get isForbidden => status == 403 || code == ApiErrorCodes.forbidden;
  bool get isNotFound => status == 404;
  bool get isNetwork =>
      code == ApiErrorCodes.networkUnavailable || code == ApiErrorCodes.timeout;
  bool get isValidation => code == ApiErrorCodes.validationFailed;

  @override
  String toString() => 'ApiException($code, $status): $message';
}

/// Server codes (see openapi.yaml) and client-side synthetic codes.
class ApiErrorCodes {
  const ApiErrorCodes._();

  static const validationFailed = 'validation_failed';
  static const unauthorized = 'unauthorized';
  static const forbidden = 'forbidden';
  static const notFound = 'not_found';
  static const conflict = 'conflict';
  static const payloadTooLarge = 'payload_too_large';
  static const unsupportedMediaType = 'unsupported_media_type';
  static const rateLimited = 'rate_limited';
  static const aiNotConfigured = 'ai_not_configured';
  static const internal = 'internal';

  // Client-side.
  static const networkUnavailable = 'network_unavailable';
  static const timeout = 'timeout';
  static const cancelled = 'cancelled';
  static const badResponse = 'bad_response';
  static const unknown = 'unknown';
}
