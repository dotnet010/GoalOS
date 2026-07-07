// SkillSystem - Grenade & Time Slow
const SkillSystem = {
  grenades: [],
  grenadeCD: 0,
  grenadeCDMax: 5,
  grenadeCount: 3,
  timeSlowCD: 0,
  timeSlowCDMax: 15,
  timeSlowDuration: 0,
  timeSlowMax: 4,
  scene: null,

  init(scene) {
    this.scene = scene;
    this.grenades = [];
    this.grenadeCD = 0;
    this.grenadeCount = 3;
    this.timeSlowCD = 0;
    this.timeSlowDuration = 0;
    window.addEventListener('keydown', e => {
      if (e.code === 'KeyQ') this.throwGrenade();
      if (e.code === 'KeyE') this.activateTimeSlow();
    });
  },

  throwGrenade() {
    if (this.grenadeCD > 0 || this.grenadeCount <= 0) return;
    this.grenadeCD = this.grenadeCDMax;
    this.grenadeCount--;
    const grenade = AssetFactory.createGrenade();
    const startPos = PlayerController.pos.clone();
    startPos.y = 0.9;
    grenade.position.copy(startPos);
    this.scene.add(grenade);
    const a = PlayerController.aimAngle;
    this.grenades.push({
      mesh: grenade, pos: startPos.clone(),
      vel: new THREE.Vector3(Math.sin(a) * 15, 8, Math.cos(a) * 15),
      timer: 1.5
    });
  },

  activateTimeSlow() {
    if (this.timeSlowCD > 0) return;
    this.timeSlowCD = this.timeSlowCDMax;
    this.timeSlowDuration = this.timeSlowMax;
    GameEngine.timeScale = 0.3;
  },

  update(realDelta, scaledDelta) {
    if (this.grenadeCD > 0) this.grenadeCD -= realDelta;
    if (this.timeSlowCD > 0) this.timeSlowCD -= realDelta;
    if (this.timeSlowDuration > 0) {
      this.timeSlowDuration -= realDelta;
      if (this.timeSlowDuration <= 0) GameEngine.timeScale = 1.0;
    }
    for (let i = this.grenades.length - 1; i >= 0; i--) {
      const g = this.grenades[i];
      g.vel.y -= 20 * scaledDelta;
      g.pos.addScaledVector(g.vel, scaledDelta);
      if (g.pos.y < 0.15) {
        g.pos.y = 0.15;
        g.vel.y *= -0.4;
        g.vel.x *= 0.7;
        g.vel.z *= 0.7;
      }
      g.mesh.position.copy(g.pos);
      g.timer -= realDelta;
      if (g.timer <= 0) {
        ParticleSystem.spawnExplosion(g.pos);
        const r = 5;
        ZombieManager.getZombiesInRange(g.pos, r).forEach(z => {
          const d = z.pos.distanceTo(g.pos);
          ZombieManager.hitZombie(z, 200 * (1 - d / r));
        });
        this.scene.remove(g.mesh);
        this.grenades.splice(i, 1);
      }
    }
  },

  reset() {
    this.grenades.forEach(g => this.scene.remove(g.mesh));
    this.grenades = [];
    this.grenadeCD = 0;
    this.grenadeCount = 3;
    this.timeSlowCD = 0;
    this.timeSlowDuration = 0;
    GameEngine.timeScale = 1.0;
  }
};