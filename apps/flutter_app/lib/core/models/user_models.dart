import 'enums.dart';
import 'json.dart';

class User {
  const User({
    required this.id,
    required this.email,
    required this.role,
    this.status = 'active',
    this.displayName = '',
    this.locale = 'tr',
    this.locationConsent = false,
    this.createdAt,
  });

  final String id;
  final String email;
  final UserRole role;
  final String status;
  final String displayName;
  final String locale;
  final bool locationConsent;
  final DateTime? createdAt;

  factory User.fromJson(Json j) => User(
        id: str(j['id']),
        email: str(j['email']),
        role: UserRole.fromWire(strOrNull(j['role'])),
        status: str(j['status'], 'active'),
        displayName: str(j['display_name']),
        locale: str(j['locale'], 'tr'),
        locationConsent: boolOf(j['location_consent']),
        createdAt: dateOrNull(j['created_at']),
      );

  Json toJson() => {
        'id': id,
        'email': email,
        'role': role.wire,
        'status': status,
        'display_name': displayName,
        'locale': locale,
        'location_consent': locationConsent,
        'created_at': isoOrNull(createdAt),
      };

  User copyWith({
    String? displayName,
    String? locale,
    bool? locationConsent,
  }) =>
      User(
        id: id,
        email: email,
        role: role,
        status: status,
        displayName: displayName ?? this.displayName,
        locale: locale ?? this.locale,
        locationConsent: locationConsent ?? this.locationConsent,
        createdAt: createdAt,
      );
}

/// Response of POST /auth/login and POST /auth/register (payload of `data`).
class AuthSession {
  const AuthSession({
    required this.token,
    required this.user,
    this.expiresAt,
  });

  final String token;
  final DateTime? expiresAt;
  final User user;

  factory AuthSession.fromJson(Json j) => AuthSession(
        token: str(j['token']),
        expiresAt: dateOrNull(j['expires_at']),
        user: User.fromJson(asMap(j['user'])),
      );

  Json toJson() => {
        'token': token,
        'expires_at': isoOrNull(expiresAt),
        'user': user.toJson(),
      };
}

class LoginRequest {
  const LoginRequest({required this.email, required this.password});
  final String email;
  final String password;

  factory LoginRequest.fromJson(Json j) =>
      LoginRequest(email: str(j['email']), password: str(j['password']));
  Json toJson() => {'email': email, 'password': password};
}

class RegisterRequest {
  const RegisterRequest({
    required this.email,
    required this.password,
    this.displayName,
  });

  final String email;
  final String password;
  final String? displayName;

  factory RegisterRequest.fromJson(Json j) => RegisterRequest(
        email: str(j['email']),
        password: str(j['password']),
        displayName: strOrNull(j['display_name']),
      );

  Json toJson() => compact({
        'email': email,
        'password': password,
        'display_name': displayName,
      });
}

/// Body of PATCH /auth/me. Only non-null fields are sent.
class ProfileUpdate {
  const ProfileUpdate({this.displayName, this.locale, this.locationConsent});
  final String? displayName;
  final String? locale;
  final bool? locationConsent;

  factory ProfileUpdate.fromJson(Json j) => ProfileUpdate(
        displayName: strOrNull(j['display_name']),
        locale: strOrNull(j['locale']),
        locationConsent:
            j['location_consent'] == null ? null : boolOf(j['location_consent']),
      );

  Json toJson() => compact({
        'display_name': displayName,
        'locale': locale,
        'consent_location': locationConsent,
      });
}

/// Body of PATCH /admin/users/{id}.
class AdminUserUpdate {
  const AdminUserUpdate({this.role, this.status});
  final UserRole? role;
  final String? status;

  factory AdminUserUpdate.fromJson(Json j) => AdminUserUpdate(
        role: j['role'] == null ? null : UserRole.fromWire(str(j['role'])),
        status: strOrNull(j['status']),
      );
  Json toJson() => compact({'role': role?.wire, 'status': status});
}
