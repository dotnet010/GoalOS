// ParticleSystem - Muzzle flash, blood splatter, explosion
const ParticleSystem = {
  particles: [],
  scene: null,

  init(scene) {
    this.scene = scene;
    this.particles = [];
  },

  spawnMuzzleFlash(pos, angle) {
    const light = new THREE.PointLight(0xffaa44, 3, 5);
    light.position.copy(pos);
    this.scene.add(light);
    this.particles.push({ type: 'light', mesh: light, life: 0.08, maxLife: 0.08 });
    for (let i = 0; i < 5; i++) {
      const p = new THREE.Mesh(
        new THREE.BoxGeometry(0.05, 0.05, 0.05),
        new THREE.MeshBasicMaterial({ color: 0xffdd66 })
      );
      p.position.copy(pos);
      this.scene.add(p);
      this.particles.push({
        type: 'spark', mesh: p,
        vel: new THREE.Vector3(
          Math.sin(angle) * 5 + (Math.random()-0.5) * 0.3,
          Math.random() * 2,
          Math.cos(angle) * 5 + (Math.random()-0.5) * 0.3
        ),
        life: 0.15, maxLife: 0.15
      });
    }
  },

  spawnBlood(pos) {
    for (let i = 0; i < 12; i++) {
      const p = new THREE.Mesh(
        new THREE.BoxGeometry(0.08, 0.08, 0.08),
        new THREE.MeshBasicMaterial({ color: 0xaa0000 })
      );
      p.position.copy(pos);
      p.position.y = 0.8;
      this.scene.add(p);
      this.particles.push({
        type: 'blood', mesh: p,
        vel: new THREE.Vector3(
          (Math.random()-0.5) * 6,
          Math.random() * 4 + 1,
          (Math.random()-0.5) * 6
        ),
        life: 0.6, maxLife: 0.6, gravity: true
      });
    }
  },

  spawnExplosion(pos) {
    const light = new THREE.PointLight(0xff6600, 8, 12);
    light.position.copy(pos);
    light.position.y = 1;
    this.scene.add(light);
    this.particles.push({ type: 'light', mesh: light, life: 0.3, maxLife: 0.3 });

    const fireColors = [0xff4400, 0xff8800, 0xffcc00, 0x660000];
    for (let i = 0; i < 25; i++) {
      const p = new THREE.Mesh(
        new THREE.BoxGeometry(0.15, 0.15, 0.15),
        new THREE.MeshBasicMaterial({ color: fireColors[Math.floor(Math.random()*fireColors.length)] })
      );
      p.position.copy(pos);
      p.position.y = 0.5;
      this.scene.add(p);
      this.particles.push({
        type: 'fire', mesh: p,
        vel: new THREE.Vector3(
          (Math.random()-0.5) * 10,
          Math.random() * 8 + 2,
          (Math.random()-0.5) * 10
        ),
        life: 0.8, maxLife: 0.8, gravity: true, scaleDown: true
      });
    }
    for (let i = 0; i < 15; i++) {
      const p = new THREE.Mesh(
        new THREE.BoxGeometry(0.2, 0.2, 0.2),
        new THREE.MeshBasicMaterial({ color: 0x333333, transparent: true, opacity: 0.7 })
      );
      p.position.copy(pos);
      p.position.y = 1;
      this.scene.add(p);
      this.particles.push({
        type: 'smoke', mesh: p,
        vel: new THREE.Vector3(
          (Math.random()-0.5) * 3,
          Math.random() * 3 + 1,
          (Math.random()-0.5) * 3
        ),
        life: 1.5, maxLife: 1.5, scaleUp: true, fadeOut: true
      });
    }
  },

  update(delta) {
    for (let i = this.particles.length - 1; i >= 0; i--) {
      const p = this.particles[i];
      p.life -= delta;
      if (p.type === 'light') {
        p.mesh.intensity = (p.life / p.maxLife) * (p.mesh.intensity > 4 ? 8 : 3);
      } else {
        if (p.vel) {
          if (p.gravity) p.vel.y -= 15 * delta;
          p.mesh.position.addScaledVector(p.vel, delta);
        }
        if (p.scaleDown) p.mesh.scale.setScalar(Math.max(0.01, p.life / p.maxLife));
        if (p.scaleUp) p.mesh.scale.setScalar(1 + (1 - p.life / p.maxLife) * 2);
        if (p.fadeOut) p.mesh.material.opacity = (p.life / p.maxLife) * 0.7;
      }
      if (p.life <= 0) {
        this.scene.remove(p.mesh);
        this.particles.splice(i, 1);
      }
    }
  },

  reset() {
    this.particles.forEach(p => this.scene.remove(p.mesh));
    this.particles = [];
  }
};