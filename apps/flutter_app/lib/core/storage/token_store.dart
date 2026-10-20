import 'package:flutter/foundation.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:shared_preferences/shared_preferences.dart';

/// Persists the opaque bearer token.
abstract class TokenStore {
  Future<String?> read();
  Future<void> write(String token);
  Future<void> clear();
}

/// In-memory store (tests, or when nothing else works).
class MemoryTokenStore implements TokenStore {
  MemoryTokenStore([this._token]);
  String? _token;

  @override
  Future<String?> read() async => _token;
  @override
  Future<void> write(String token) async => _token = token;
  @override
  Future<void> clear() async => _token = null;
}

/// Uses the platform keystore through flutter_secure_storage (libsecret on
/// Linux, Keychain on Apple, EncryptedSharedPreferences on Android). If the
/// keystore is unavailable (e.g. no Secret Service daemon on a headless Linux
/// box) it falls back to SharedPreferences: weaker at-rest protection, but the
/// token is only an opaque server-revocable session token.
class SecureTokenStore implements TokenStore {
  SecureTokenStore(this._prefs, [FlutterSecureStorage? secure])
      : _secure = secure ?? const FlutterSecureStorage();

  static const _key = 'auth_token';
  final SharedPreferences _prefs;
  final FlutterSecureStorage _secure;

  @override
  Future<String?> read() async {
    try {
      final v = await _secure.read(key: _key);
      if (v != null && v.isNotEmpty) return v;
    } catch (e) {
      debugPrint('SecureTokenStore.read fallback: $e');
    }
    return _prefs.getString(_key);
  }

  @override
  Future<void> write(String token) async {
    try {
      await _secure.write(key: _key, value: token);
      await _prefs.remove(_key);
      return;
    } catch (e) {
      debugPrint('SecureTokenStore.write fallback to prefs: $e');
    }
    await _prefs.setString(_key, token);
  }

  @override
  Future<void> clear() async {
    try {
      await _secure.delete(key: _key);
    } catch (e) {
      debugPrint('SecureTokenStore.clear: $e');
    }
    await _prefs.remove(_key);
  }
}
