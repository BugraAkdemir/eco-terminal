import 'package:eco_terminal/core/models/models.dart';
import 'package:eco_terminal/core/router/route_access.dart';
import 'package:eco_terminal/core/session/session_controller.dart';
import 'package:eco_terminal/features/maps/domain/geo.dart';
import 'package:eco_terminal/features/maps/domain/map_state.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:latlong2/latlong.dart';

SessionState authed(UserRole role) => SessionState(
      status: SessionStatus.authenticated,
      token: 't',
      user: User(id: 'u', email: 'a@b.co', role: role),
    );

const anon = SessionState(status: SessionStatus.anonymous);

String? redirect(SessionState s, String location) => computeRedirect(
      session: s,
      location: Uri.parse(location),
      accessFor: defaultAccessFor,
    );

void main() {
  group('route guards', () {
    test('restoring session parks on splash, then leaves it', () {
      expect(redirect(const SessionState(), '/'), AppPaths.splash);
      expect(redirect(anon, AppPaths.splash), AppPaths.home);
    });

    test('anonymous users browse the map but are sent to login for protected pages',
        () {
      expect(redirect(anon, '/'), isNull);
      expect(redirect(anon, '/observations/abc'), isNull);
      expect(redirect(anon, '/profile'), startsWith('/login?from='));
      expect(redirect(anon, '/observations/new'), startsWith('/login?from='));
      expect(redirect(anon, '/expert'), startsWith('/login?from='));
    });

    test('role-aware: expert area needs reviewer or admin', () {
      expect(redirect(authed(UserRole.citizen), '/expert'), AppPaths.forbidden);
      expect(redirect(authed(UserRole.researcher), '/expert'), AppPaths.forbidden);
      expect(redirect(authed(UserRole.reviewer), '/expert'), isNull);
      expect(redirect(authed(UserRole.admin), '/expert'), isNull);
      expect(redirect(authed(UserRole.citizen), '/profile'), isNull);
    });

    test('authenticated users skip the login page', () {
      expect(redirect(authed(UserRole.citizen), '/login'), AppPaths.home);
      expect(
        redirect(authed(UserRole.citizen), '/login?from=%2Fprofile'),
        '/profile',
      );
    });

    test('a 401 (expired session) redirects to login once', () {
      const expired = SessionState(status: SessionStatus.anonymous, expired: true);
      expect(redirect(expired, '/'), '/login?expired=1');
      expect(redirect(expired, '/login?expired=1'), isNull);
    });
  });

  group('selectedRegion shared context', () {
    test('map selection updates the provider and listeners see it', () {
      final container = ProviderContainer();
      addTearDown(container.dispose);
      final seen = <String?>[];
      container.listen<Region?>(
        selectedRegionProvider,
        (_, next) => seen.add(next?.id),
      );
      expect(container.read(selectedRegionProvider), isNull);

      // A tap inside a district polygon picks the finest region containing it.
      final province = RegionShape(
        const Region(id: 'prov', kind: RegionKind.province, name: 'Kastamonu'),
        [GeoPolygon(outer: _square(33, 41, 35, 43))],
      );
      final district = RegionShape(
        const Region(id: 'dist', kind: RegionKind.district, name: 'Merkez'),
        [GeoPolygon(outer: _square(33.5, 41.5, 34, 42))],
      );
      final hit = pickRegionAt(const LatLng(41.7, 33.7), [province, district]);
      expect(hit?.region.id, 'dist');
      container.read(selectedRegionProvider.notifier).select(hit!.region);
      expect(container.read(selectedRegionProvider)?.name, 'Merkez');

      container.read(selectedRegionProvider.notifier).clear();
      expect(seen, ['dist', null]);
    });

    test('polygon holes and MultiPolygon parsing', () {
      final poly = parseGeometry({
        'type': 'MultiPolygon',
        'coordinates': [
          [
            [
              [0, 0],
              [10, 0],
              [10, 10],
              [0, 10],
              [0, 0],
            ],
            [
              [4, 4],
              [6, 4],
              [6, 6],
              [4, 6],
              [4, 4],
            ],
          ],
        ],
      });
      expect(poly, hasLength(1));
      expect(poly.single.contains(const LatLng(1, 1)), isTrue);
      expect(poly.single.contains(const LatLng(5, 5)), isFalse, reason: 'hole');
      expect(poly.single.contains(const LatLng(20, 20)), isFalse);
    });
  });
}

List<LatLng> _square(double w, double s, double e, double n) =>
    [LatLng(s, w), LatLng(s, e), LatLng(n, e), LatLng(n, w)];
