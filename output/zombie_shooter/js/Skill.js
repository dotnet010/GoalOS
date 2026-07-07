class Grenade {
  constructor(pos, dir) {
    this.velocity = dir.clone().normalize().multiplyScalar(15);
    this.velocity.y = 8;
    this.timer = 1.5; this.exploded = false;
    this.mesh = new THREE.Mesh(new THREE.SphereGeometry(0.2, 8, 8), new THREE.MeshStandardMaterial({ color: 0x3a5a3a }));
    this.mesh.position.copy(pos); this.mesh.castShadow = true;
    Game.scene.add(this.mesh);
  }
  update(dt) {
    if (this.exploded) return false;
    this.timer -= dt;
    this.mesh.position.add(this.velocity.clone().multiplyScalar(dt));
    this.velocity.y -= 20 * dt;
    if (this.mesh.position.y <= 0.2) { this.mesh.position.y = 0.2; this.velocity.y *= -0.4; this.velocity.x *= 0.7; this.velocity.z *= 0.7; }
    this.mesh.rotation.x += dt * 5; this.mesh.rotation.z += dt * 3;
    this.mesh.material.color.setHex(Math.sin(this.timer * 20) > 0 ? 0xff0000 : 0x3a5a3a);
    if (this.timer <= 0) { this.explode(); return false; }
    return true;
  }
  explode() {
    this.exploded = true;
    const pos = this.mesh.position.clone();
    Game.particleSystem.explosion(pos);
    Game.scene.remove(this.mesh);
    const radius = 6;
    for (let i = Game.zombies.length - 1; i >= 0; i--) {
      const z = Game.zombies[i];
      if (!z.alive) continue;
      const dx = z.mesh.position.x - pos.x, dz = z.mesh.position.z - pos.z;
      const dist = Math.sqrt(dx * dx + dz * dz);
      if (dist < radius) z.takeDamage(100 * (1 - dist / radius));
    }
  }
  destroy() { Game.scene.remove(this.mesh); }
}
class SkillSystem {
  constructor() {
    this.grenadeCD = 0; this.grenadeMaxCD = 5;
    this.timeSlowCD = 0; this.timeSlowMaxCD = 12;
    this.timeSlowActive = false; this.timeSlowTimer = 0; this.timeSlowDuration = 4;
    this.qp = false; this.ep = false;
  }
  activateGrenade() {
    if (this.grenadeCD > 0) return;
    Game.grenades.push(new Grenade(Game.player.getMuzzlePos(), Game.player.getAimDir()));
    this.grenadeCD = this.grenadeMaxCD;
  }
  activateTimeSlow() {
    if (this.timeSlowCD > 0) return;
    this.timeSlowActive = true; this.timeSlowTimer = this.timeSlowDuration;
    Game.timeScale = 0.25; this.timeSlowCD = this.timeSlowMaxCD;
  }
  update(dt) {
    if (this.grenadeCD > 0) this.grenadeCD -= dt;
    if (this.timeSlowCD > 0) this.timeSlowCD -= dt;
    if (this.timeSlowActive) { this.timeSlowTimer -= dt; if (this.timeSlowTimer <= 0) { this.timeSlowActive = false; Game.timeScale = 1.0; } }
    if (Game.keys['KeyQ'] && !this.qp) { this.activateGrenade(); this.qp = true; } if (!Game.keys['KeyQ']) this.qp = false;
    if (Game.keys['KeyE'] && !this.ep) { this.activateTimeSlow(); this.ep = true; } if (!Game.keys['KeyE']) this.ep = false;
    for (let i = Game.grenades.length - 1; i >= 0; i--) {
      if (!Game.grenades[i].update(dt)) { Game.grenades[i].destroy(); Game.grenades.splice(i, 1); }
    }
  }
}