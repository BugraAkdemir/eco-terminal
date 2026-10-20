import '../../../core/l10n/strings.dart';

/// Pure form validators (return a localized message or null).
class AuthValidators {
  const AuthValidators._();

  static const minPasswordLength = 10;

  static final _email = RegExp(r'^[^\s@]+@[^\s@]+\.[^\s@]{2,}$');

  static String? email(AppStrings s, String? v) {
    final t = (v ?? '').trim();
    if (t.isEmpty) return s.required;
    if (!_email.hasMatch(t)) return s.emailInvalid;
    return null;
  }

  static String? password(AppStrings s, String? v) {
    final t = v ?? '';
    if (t.isEmpty) return s.required;
    if (t.length < minPasswordLength) return s.passwordTooShort;
    return null;
  }

  /// Login only checks presence (the server owns credential rules).
  static String? passwordPresent(AppStrings s, String? v) =>
      (v == null || v.isEmpty) ? s.required : null;
}
