// WeaponSystem - 3 weapons, projectile physics, damage
const WeaponSystem = {
  currentWeapon: 0,
  weapons: [
    { name: 'Pistol',  damage: 25, fireRate: 0.35, speed: 40, spread: 0.02, pellets: 1 },
    { name: 'Shotgun', damage: 18, fireRate: 0.80, speed: 30, spread: 0.15, pellets: 6 },
    { name: 'Rifle',   damage: 15, fireRate: 0.10, speed: 50, spread: 0.04, pellets: 1 }
  ],
  projectiles: [],
  scene: null,
  fireTimer: 0,
  muzzlePos: new THREE.Vector3(),

  init(scene) {
    this.scene = scene;
    this.projectiles = [];
    this.currentWeapon = 0;
    this.fireTimer = 0;
    window.addEventListener('keydown', e => {
      if (e.code === 'Digit1') this.currentWeapon = 0;
      if (e.code === 'Digit2') this.currentWeapon = 1;
      if (e.code === 'Digit3') this.currentWeapon = 2;
    });
  },

  fire(playerPos, aimAngle, isFiring) {
    this.fireTimer -= GameEngine.scaledDelta;
    const w = this.weapons[this.currentWeapon];
    if (!isFiring || this.fireTimer > 0) return;
    this.fireTimer = w.fireRate;
    this.muzzlePos.set(
      playerPos.x + Math.sin(aimAngle) * 0.5,
      0.9,
      playerPos.z + Math.cos(aimAngle) * 0.5
    );
    for (let i = 0; i < w.pellets; i++) {
      const spread = (Math.random() - 0.5) * w.spread * 2;
      const angle = aimAngle + spread;
      const bullet = AssetFactory.createBullet();
      bullet.position.copy(this.muzzlePos);
      this.scene.add(bullet);
      this.projectiles.push({
        mesh: bullet, pos: this.muzzlePos.clone(),
        vel: new THREE.Vector3(Math.sin(angle) * w.speed, 0, Math.cos(angle) * w.speed),
        damage: w.damage, life: 2.0, radius: 0.08
      });
    }
    ParticleSystem.spawnMuzzleFlash(this.muzzlePos, aimAngle);
  },

  update(delta) {
    for (let i = this.projectiles.length - 1; i >= 0; i--) {
      const p = this.projectiles[i];
      p.pos.addScaledVector(p.vel, delta);
      p.mesh.position.copy(p.pos);
      p.life -= delta;
      let hit = false;
      for (const z of ZombieManager.zombies) {
        if (p.pos.distanceTo(z.pos) < p.radius + z.radius) {
          ZombieManager.hitZombie(z, p.damage);
          ParticleSystem.spawnBlood(p.pos);
          hit = true;
          break;
        }
      }
      if (hit || p.life <= 0 || Math.abs(p.pos.x) > 50 || Math.abs(p.pos.z) > 50) {
        this.scene.remove(p.mesh);
        this.projectiles.splice(i, 1);
      }
    }
  },

  reset() {
    this.projectiles.forEach(p => this.scene.remove(p.mesh));
    this.projectiles = [];
    this.currentWeapon = 0;
    this.fireTimer = 0;
  }
};