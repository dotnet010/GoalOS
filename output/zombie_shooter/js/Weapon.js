const WeaponConfig = {
  pistol:  { name: 'Pistol',  damage: 25, fireRate: 0.35, spread: 0.02, pellets: 1, maxAmmo: Infinity, reloadTime: 0,   auto: false },
  shotgun: { name: 'Shotgun', damage: 12, fireRate: 0.8,  spread: 0.15, pellets: 8, maxAmmo: 24,         reloadTime: 1.5, auto: false },
  rifle:   { name: 'Rifle',   damage: 15, fireRate: 0.1,  spread: 0.04, pellets: 1, maxAmmo: 30,         reloadTime: 1.8, auto: true  }
};
class WeaponSystem {
  constructor() {
    this.current = 'pistol';
    this.fireCooldown = 0; this.reloading = false; this.reloadTimer = 0;
    this.ammo = { pistol: Infinity, shotgun: 24, rifle: 30 };
    this.lastFired = false;
    this.d1p = this.d2p = this.d3p = this.rp = false;
  }
  switchWeapon(name) {
    if (!WeaponConfig[name] || this.reloading) return;
    this.current = name; this.fireCooldown = 0.2;
    Game.ui.updateWeaponDisplay(); Game.ui.updateAmmoDisplay();
  }
  fire() {
    if (this.fireCooldown > 0 || this.reloading) return;
    const cfg = WeaponConfig[this.current];
    if (this.ammo[this.current] !== Infinity && this.ammo[this.current] <= 0) { this.reload(); return; }
    const muzzle = Game.player.getMuzzlePos();
    const baseDir = Game.player.getAimDir();
    for (let i = 0; i < cfg.pellets; i++) {
      const a = Math.atan2(baseDir.x, baseDir.z) + (Math.random() - 0.5) * cfg.spread * 2;
      Game.projectiles.push(new Bullet(muzzle, new THREE.Vector3(Math.sin(a), 0, Math.cos(a)), cfg.damage, 'player'));
    }
    Game.particleSystem.muzzleFlash(muzzle, baseDir);
    if (this.ammo[this.current] !== Infinity) this.ammo[this.current]--;
    this.fireCooldown = cfg.fireRate;
    Game.ui.updateAmmoDisplay();
  }
  reload() {
    const cfg = WeaponConfig[this.current];
    if (cfg.reloadTime === 0 || this.reloading || this.ammo[this.current] === cfg.maxAmmo) return;
    this.reloading = true; this.reloadTimer = cfg.reloadTime;
    Game.ui.updateAmmoDisplay();
  }
  update(dt) {
    if (this.fireCooldown > 0) this.fireCooldown -= dt;
    if (this.reloading) {
      this.reloadTimer -= dt;
      if (this.reloadTimer <= 0) { this.reloading = false; this.ammo[this.current] = WeaponConfig[this.current].maxAmmo; Game.ui.updateAmmoDisplay(); }
    }
    if (Game.keys['Digit1'] && !this.d1p) { this.switchWeapon('pistol'); this.d1p = true; } if (!Game.keys['Digit1']) this.d1p = false;
    if (Game.keys['Digit2'] && !this.d2p) { this.switchWeapon('shotgun'); this.d2p = true; } if (!Game.keys['Digit2']) this.d2p = false;
    if (Game.keys['Digit3'] && !this.d3p) { this.switchWeapon('rifle'); this.d3p = true; } if (!Game.keys['Digit3']) this.d3p = false;
    if (Game.keys['KeyR'] && !this.rp) { this.reload(); this.rp = true; } if (!Game.keys['KeyR']) this.rp = false;
    const cfg = WeaponConfig[this.current];
    if (cfg.auto && Game.mouse.down) this.fire();
    else if (Game.mouse.down && !this.lastFired) this.fire();
    this.lastFired = Game.mouse.down;
  }
}