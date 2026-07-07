class Bullet {
  constructor(pos, dir, damage, owner) {
    this.velocity = dir.clone().normalize().multiplyScalar(60);
    this.damage = damage;
    this.owner = owner;
    this.life = 2.0;
    this.radius = 0.15;
    this.mesh = new THREE.Mesh(new THREE.SphereGeometry(this.radius, 6, 6), new THREE.MeshBasicMaterial({ color: 0xffff00 }));
    this.mesh.position.copy(pos);
    Game.scene.add(this.mesh);
  }
  update(dt) {
    this.mesh.position.add(this.velocity.clone().multiplyScalar(dt));
    this.life -= dt;
    for (let i = 0; i < Game.zombies.length; i++) {
      const z = Game.zombies[i];
      if (!z.alive) continue;
      const dx = this.mesh.position.x - z.mesh.position.x;
      const dz = this.mesh.position.z - z.mesh.position.z;
      if (Math.sqrt(dx * dx + dz * dz) < z.radius + this.radius) {
        z.takeDamage(this.damage);
        Game.particleSystem.bloodSplatter(this.mesh.position.clone());
        return false;
      }
    }
    const lim = Game.WORLD_SIZE;
    if (Math.abs(this.mesh.position.x) > lim || Math.abs(this.mesh.position.z) > lim) return false;
    return this.life > 0;
  }
  destroy() {
    Game.scene.remove(this.mesh);
    this.mesh.geometry.dispose();
    this.mesh.material.dispose();
  }
}