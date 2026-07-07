const ZombieTypes = {
  normal: { health: 30, speed: 3, damage: 10, radius: 0.5, score: 10, color: 0x4a7a3a, scale: 1.0 },
  fast:   { health: 15, speed: 6, damage: 5,  radius: 0.4, score: 15, color: 0x8a3a3a, scale: 0.8 },
  large:  { health: 100, speed: 1.5, damage: 25, radius: 0.9, score: 30, color: 0x3a3a5a, scale: 1.6 }
};
class Zombie {
  constructor(type, pos) {
    const cfg = ZombieTypes[type];
    this.type = type;
    this.health = cfg.health; this.maxHealth = cfg.health;
    this.speed = cfg.speed; this.damage = cfg.damage;
    this.radius = cfg.radius; this.scoreValue = cfg.score;
    this.attackCooldown = 0; this.alive = true; this.walkPhase = Math.random() * Math.PI * 2;
    const group = new THREE.Group();
    const body = new THREE.Mesh(new THREE.CylinderGeometry(0.35 * cfg.scale, 0.3 * cfg.scale, 1.0 * cfg.scale, 8), new THREE.MeshStandardMaterial({ color: cfg.color, roughness: 0.8 }));
    body.position.y = 0.7 * cfg.scale; body.castShadow = true; group.add(body);
    this.body = body;
    const head = new THREE.Mesh(new THREE.SphereGeometry(0.28 * cfg.scale, 8, 8), new THREE.MeshStandardMaterial({ color: 0x6a8a5a, roughness: 0.7 }));
    head.position.y = 1.4 * cfg.scale; head.castShadow = true; group.add(head);
    const eyeGeo = new THREE.SphereGeometry(0.06 * cfg.scale, 4, 4);
    const eyeMat = new THREE.MeshBasicMaterial({ color: 0xff0000 });
    [-0.1, 0.1].forEach(ex => { const e = new THREE.Mesh(eyeGeo, eyeMat); e.position.set(ex * cfg.scale, 1.45 * cfg.scale, 0.22 * cfg.scale); group.add(e); });
    this.mesh = group;
    this.mesh.position.copy(pos);
    Game.scene.add(this.mesh);
    this.barBg = new THREE.Mesh(new THREE.PlaneGeometry(1.0, 0.12), new THREE.MeshBasicMaterial({ color: 0x330000, depthTest: false, transparent: true }));
    this.barBg.renderOrder = 999;
    Game.scene.add(this.barBg);
    this.bar = new THREE.Mesh(new THREE.PlaneGeometry(1.0, 0.12), new THREE.MeshBasicMaterial({ color: 0xff0000, depthTest: false, transparent: true }));
    this.bar.renderOrder = 1000;
    Game.scene.add(this.bar);
  }
  update(dt) {
    if (!this.alive) return;
    const sdt = dt * Game.timeScale;
    const dx = Game.player.mesh.position.x - this.mesh.position.x;
    const dz = Game.player.mesh.position.z - this.mesh.position.z;
    const dist = Math.sqrt(dx * dx + dz * dz);
    if (dist > 0.01) {
      this.mesh.position.x += (dx / dist) * this.speed * sdt;
      this.mesh.position.z += (dz / dist) * this.speed * sdt;
      this.mesh.rotation.y = Math.atan2(dx, dz);
    }
    this.walkPhase += sdt * 5;
    this.body.position.y = 0.7 * ZombieTypes[this.type].scale + Math.sin(this.walkPhase) * 0.05;
    this.attackCooldown -= dt;
    if (dist < this.radius + Game.player.radius + 0.3 && this.attackCooldown <= 0) {
      Game.player.takeDamage(this.damage);
      this.attackCooldown = 1.0;
    }
    const by = 2.0 * ZombieTypes[this.type].scale;
    this.barBg.position.set(this.mesh.position.x, by, this.mesh.position.z);
    this.barBg.rotation.x = -Math.PI / 2;
    this.bar.position.set(this.mesh.position.x, by + 0.01, this.mesh.position.z);
    this.bar.rotation.x = -Math.PI / 2;
    this.bar.scale.x = Math.max(0, this.health / this.maxHealth);
    for (let i = 0; i < Game.zombies.length; i++) {
      const o = Game.zombies[i];
      if (o === this || !o.alive) continue;
      const ox = this.mesh.position.x - o.mesh.position.x;
      const oz = this.mesh.position.z - o.mesh.position.z;
      const od = Math.sqrt(ox * ox + oz * oz);
      const minD = this.radius + o.radius;
      if (od < minD && od > 0.01) {
        const push = (minD - od) * 0.5;
        this.mesh.position.x += (ox / od) * push * sdt * 10;
        this.mesh.position.z += (oz / od) * push * sdt * 10;
      }
    }
  }
  takeDamage(amount) {
    this.health -= amount;
    if (this.health <= 0) { this.health = 0; this.die(); }
  }
  die() {
    this.alive = false;
    Game.addScore(this.scoreValue);
    Game.particleSystem.bloodSplatter(this.mesh.position.clone());
    Game.scene.remove(this.mesh);
    Game.scene.remove(this.barBg);
    Game.scene.remove(this.bar);
  }
}