class ParticleSystem {
  constructor() { this.particles = []; }
  muzzleFlash(pos, dir) {
    const light = new THREE.PointLight(0xffaa00, 3, 10);
    light.position.copy(pos);
    Game.scene.add(light);
    const flash = new THREE.Mesh(new THREE.SphereGeometry(0.3, 6, 6), new THREE.MeshBasicMaterial({ color: 0xffdd44, transparent: true }));
    flash.position.copy(pos);
    Game.scene.add(flash);
    this.particles.push({ mesh: flash, light: light, life: 0.08, maxLife: 0.08, update(dt) { this.life -= dt; const t = this.life / this.maxLife; this.mesh.scale.setScalar(1 + (1 - t) * 2); this.mesh.material.opacity = t; this.light.intensity = 3 * t; } });
    for (let i = 0; i < 5; i++) {
      const spark = new THREE.Mesh(new THREE.SphereGeometry(0.05, 4, 4), new THREE.MeshBasicMaterial({ color: 0xffaa00, transparent: true }));
      spark.position.copy(pos);
      Game.scene.add(spark);
      const vel = new THREE.Vector3(dir.x + (Math.random() - 0.5) * 2, Math.random() * 2, dir.z + (Math.random() - 0.5) * 2).normalize().multiplyScalar(3 + Math.random() * 3);
      this.particles.push({ mesh: spark, life: 0.3, maxLife: 0.3, velocity: vel, update(dt) { this.life -= dt; this.mesh.position.add(this.velocity.clone().multiplyScalar(dt)); this.velocity.y -= 10 * dt; this.mesh.material.opacity = this.life / this.maxLife; } });
    }
  }
  bloodSplatter(pos) {
    for (let i = 0; i < 15; i++) {
      const m = new THREE.Mesh(new THREE.SphereGeometry(0.08 + Math.random() * 0.1, 4, 4), new THREE.MeshBasicMaterial({ color: 0x660000 + Math.floor(Math.random() * 0x222222), transparent: true }));
      m.position.copy(pos); m.position.y += 0.5;
      Game.scene.add(m);
      const a = Math.random() * Math.PI * 2, s = 2 + Math.random() * 4;
      const vel = new THREE.Vector3(Math.cos(a) * s, 1 + Math.random() * 3, Math.sin(a) * s);
      this.particles.push({ mesh: m, life: 0.6 + Math.random() * 0.4, maxLife: 1.0, velocity: vel, update(dt) { this.life -= dt; this.mesh.position.add(this.velocity.clone().multiplyScalar(dt)); this.velocity.y -= 12 * dt; if (this.mesh.position.y < 0.05) { this.mesh.position.y = 0.05; this.velocity.set(0, 0, 0); } this.mesh.material.opacity = Math.max(0, this.life / this.maxLife); } });
    }
  }
  explosion(pos) {
    const light = new THREE.PointLight(0xff6600, 8, 25);
    light.position.copy(pos); light.position.y = 2;
    Game.scene.add(light);
    const sphere = new THREE.Mesh(new THREE.SphereGeometry(0.5, 12, 12), new THREE.MeshBasicMaterial({ color: 0xff6600, transparent: true }));
    sphere.position.copy(pos); sphere.position.y = 1;
    Game.scene.add(sphere);
    this.particles.push({ mesh: sphere, light: light, life: 0.5, maxLife: 0.5, update(dt) { this.life -= dt; const t = 1 - this.life / this.maxLife; this.mesh.scale.setScalar(1 + t * 8); this.mesh.material.opacity = (1 - t) * 0.8; this.light.intensity = 8 * (1 - t); } });
    for (let i = 0; i < 30; i++) {
      const colors = [0xff4400, 0xff6600, 0xff8800, 0x666666];
      const m = new THREE.Mesh(new THREE.BoxGeometry(0.15, 0.15, 0.15), new THREE.MeshBasicMaterial({ color: colors[Math.floor(Math.random() * colors.length)], transparent: true }));
      m.position.copy(pos); m.position.y = 1;
      Game.scene.add(m);
      const a = Math.random() * Math.PI * 2, s = 5 + Math.random() * 10;
      const vel = new THREE.Vector3(Math.cos(a) * s, 2 + Math.random() * 8, Math.sin(a) * s);
      this.particles.push({ mesh: m, life: 0.8 + Math.random() * 0.5, maxLife: 1.3, velocity: vel, update(dt) { this.life -= dt; this.mesh.position.add(this.velocity.clone().multiplyScalar(dt)); this.velocity.y -= 15 * dt; if (this.mesh.position.y < 0.1) { this.mesh.position.y = 0.1; this.velocity.y *= -0.3; this.velocity.x *= 0.7; this.velocity.z *= 0.7; } this.mesh.material.opacity = Math.max(0, this.life / this.maxLife); this.mesh.rotation.x += dt * 5; this.mesh.rotation.z += dt * 5; } });
    }
  }
  update(dt) {
    for (let i = this.particles.length - 1; i >= 0; i--) {
      const p = this.particles[i];
      p.update(dt);
      if (p.life <= 0) {
        Game.scene.remove(p.mesh);
        if (p.light) Game.scene.remove(p.light);
        p.mesh.geometry.dispose(); p.mesh.material.dispose();
        this.particles.splice(i, 1);
      }
    }
  }
}