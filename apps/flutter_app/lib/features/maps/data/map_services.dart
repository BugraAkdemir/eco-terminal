import 'package:flutter_map/flutter_map.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:geolocator/geolocator.dart';
import 'package:latlong2/latlong.dart';

import '../../../core/config/app_config.dart';

enum LocationOutcome { ok, denied, deniedForever, serviceDisabled, error }

class LocationResult {
  const LocationResult(this.outcome, [this.position]);
  final LocationOutcome outcome;
  final LatLng? position;
}

/// Device location behind an interface so tests never touch the platform.
abstract class LocationService {
  Future<LocationResult> current();
}

class GeolocatorLocationService implements LocationService {
  const GeolocatorLocationService();

  @override
  Future<LocationResult> current() async {
    try {
      if (!await Geolocator.isLocationServiceEnabled()) {
        return const LocationResult(LocationOutcome.serviceDisabled);
      }
      var perm = await Geolocator.checkPermission();
      if (perm == LocationPermission.denied) {
        perm = await Geolocator.requestPermission();
      }
      if (perm == LocationPermission.denied) {
        return const LocationResult(LocationOutcome.denied);
      }
      if (perm == LocationPermission.deniedForever) {
        return const LocationResult(LocationOutcome.deniedForever);
      }
      final pos = await Geolocator.getCurrentPosition(
        locationSettings: const LocationSettings(
          accuracy: LocationAccuracy.high,
          timeLimit: Duration(seconds: 12),
        ),
      );
      return LocationResult(
        LocationOutcome.ok,
        LatLng(pos.latitude, pos.longitude),
      );
    } catch (_) {
      // Includes plugin/platform failures (e.g. no GeoClue on Linux).
      return const LocationResult(LocationOutcome.error);
    }
  }
}

final locationServiceProvider =
    Provider<LocationService>((ref) => const GeolocatorLocationService());

/// OSM raster tiles over the network. Tests override this with a blank
/// provider. NOTE: the OSM public tile servers are for light use only; set
/// `--dart-define=TILE_URL=...` to a proper provider in production.
final tileProviderProvider = Provider<TileProvider>((ref) {
  final p = NetworkTileProvider();
  ref.onDispose(p.dispose);
  return p;
});

/// Tile URL template (overridable by dart-define).
final tileUrlProvider =
    Provider<String>((ref) => MapConfig.tileUrlTemplate);
