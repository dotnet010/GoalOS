// AssetFactory - Low Poly procedural mesh generation (Synty Studios style)
const AssetFactory = {
  createGround(size = 100) {
    const geo = new THREE.PlaneGeometry(size, size, 40, 40);
    const pos = geo.attributes.position;
    for (let i = 0; i < pos.count; i++) {
      pos.setZ(i, (Math.random() - 0.5) * 0.4);
    }
    geo.computeVertexNormals();
    const mat = new THREE.MeshLambertMaterial({ color: 0x4a7c3a, flatShading: true });
    const mesh = new THREE.Mesh(geo, mat);
    mesh.rotation.x = -Math.PI / 2;
    mesh.receiveShadow = true;
    return mesh;
  },

  createPlayer() {
    const g = new THREE.Group();
    const bodyMat = new THREE.MeshLambertMaterial({ color: 0x2a5f8a, flatShading: true });
    const skinMat = new THREE.MeshLambertMaterial({ color: 0xe8b890, flatShading: true });
    const packMat = new THREE.MeshLambertMaterial({ color: 0x5a3a2a, flatShading: true });
    const gunMat = new THREE.MeshLambertMaterial({ color: 0x222222, flatShading: true });

    const body = new THREE.Mesh(new THREE.BoxGeometry(0.55, 0.7, 0.35), bodyMat);
    body.position.y = 0.7;
    body.castShadow = true;
    g.add(body);

    const head = new THREE.Mesh(new THREE.BoxGeometry(0.3, 0.3, 0.3), skinMat);
    head.position.y = 1.25;
    head.castShadow = true;
    g.add(head);

    const pack = new THREE.Mesh(new THREE.BoxGeometry(0.3, 0.4, 0.2), packMat);
    pack.position.set(0, 0.75, -0.25);
    g.add(pack);

    const gun = new THREE.Mesh(new THREE.BoxGeometry(0.12, 0.12, 0.45), gunMat);
    gun.position.set(0.2, 0.85, 0.3);
    gun.name = 'gun';
    g.add(gun);

    g.userData = { radius: 0.4 };
    return g;
  },

  createZombie(type) {
    const g = new THREE.Group();
    const cfgs = {
      normal: { color: 0x6b8f3a, scale: 1.0, hp: 100, speed: 1.5, dmg: 10 },
      fast:   { color: 0xa04848, scale: 0.75, hp: 50, speed: 3.2, dmg: 8 },
      large:  { color: 0x3a5a3a, scale: 1.7, hp: 350, speed: 0.8, dmg: 25 }
    };
    const cfg = cfgs[type] || cfgs.normal;
    const mat = new THREE.MeshLambertMaterial({ color: cfg.color, flatShading: true });
    const s = cfg.scale;

    const body = new THREE.Mesh(new THREE.BoxGeometry(0.5*s, 0.65*s, 0.3*s), mat);
    body.position.y = 0.65*s;
    body.castShadow = true;
    g.add(body);

    const head = new THREE.Mesh(new THREE.BoxGeometry(0.28*s, 0.28*s, 0.28*s), mat);
    head.position.y = 1.15*s;
    head.castShadow = true;
    g.add(head);

    const eyeMat = new THREE.MeshBasicMaterial({ color: 0xff0000 });
    const eyeGeo = new THREE.BoxGeometry(0.06*s, 0.06*s, 0.06*s);
    const eyeL = new THREE.Mesh(eyeGeo, eyeMat);
    eyeL.position.set(-0.08*s, 1.18*s, 0.15*s);
    g.add(eyeL);
    const eyeR = new THREE.Mesh(eyeGeo, eyeMat);
    eyeR.position.set(0.08*s, 1.18*s, 0.15*s);
    g.add(eyeR);

    const armGeo = new THREE.BoxGeometry(0.12*s, 0.35*s, 0.12*s);
    const armL = new THREE.Mesh(armGeo, mat);
    armL.position.set(-0.32*s, 0.75*s, 0.22*s);
    armL.rotation.x = -Math.PI/3;
    armL.castShadow = true;
    g.add(armL);
    const armR = new THREE.Mesh(armGeo, mat);
    armR.position.set(0.32*s, 0.75*s, 0.22*s);
    armR.rotation.x = -Math.PI/3;
    armR.castShadow = true;
    g.add(armR);

    const legGeo = new THREE.BoxGeometry(0.15*s, 0.5*s, 0.15*s);
    const legL = new THREE.Mesh(legGeo, mat);
    legL.position.set(-0.13*s, 0.25*s, 0);
    legL.castShadow = true;
    g.add(legL);
    const legR = new THREE.Mesh(legGeo, mat);
    legR.position.set(0.13*s, 0.25*s, 0);
    legR.castShadow = true;
    g.add(legR);

    g.userData = { type, maxHp: cfg.hp, hp: cfg.hp, speed: cfg.speed, dmg: cfg.dmg, scale: s, radius: 0.4*s };
    return g;
  },

  createEnvironment() {
    const g = new THREE.Group();
    for (let i = 0; i < 40; i++) {
      const x = (Math.random() - 0.5) * 90;
      const z = (Math.random() - 0.5) * 90;
      if (Math.abs(x) < 8 && Math.abs(z) < 8) continue;

      if (Math.random() > 0.4) {
        const tree = new THREE.Group();
        const trunk = new THREE.Mesh(
          new THREE.CylinderGeometry(0.12, 0.18, 1.0, 6),
          new THREE.MeshLambertMaterial({ color: 0x4a3020, flatShading: true })
        );
        trunk.position.y = 0.5;
        trunk.castShadow = true;
        tree.add(trunk);
        const leaves = new THREE.Mesh(
          new THREE.ConeGeometry(0.55, 1.3, 6),
          new THREE.MeshLambertMaterial({ color: 0x2d5a1d, flatShading: true })
        );
        leaves.position.y = 1.5;
        leaves.castShadow = true;
        tree.add(leaves);
        tree.position.set(x, 0, z);
        tree.scale.setScalar(0.8 + Math.random() * 0.5);
        tree.userData = { radius: 0.5, solid: true };
        g.add(tree);
      } else {
        const rock = new THREE.Mesh(
          new THREE.DodecahedronGeometry(0.25 + Math.random() * 0.35, 0),
          new THREE.MeshLambertMaterial({ color: 0x777777, flatShading: true })
        );
        rock.position.set(x, 0.15, z);
        rock.rotation.set(Math.random()*Math.PI, Math.random()*Math.PI, Math.random()*Math.PI);
        rock.castShadow = true;
        rock.userData = { radius: 0.4, solid: true };
        g.add(rock);
      }
    }
    const wallMat = new THREE.MeshLambertMaterial({ color: 0x554433, flatShading: true });
    const walls = [
      { pos: [0, 0.75, -50], size: [100, 1.5, 0.5] },
      { pos: [0, 0.75, 50], size: [100, 1.5, 0.5] },
      { pos: [-50, 0.75, 0], size: [0.5, 1.5, 100] },
      { pos: [50, 0.75, 0], size: [0.5, 1.5, 100] }
    ];
    walls.forEach(w => {
      const wall = new THREE.Mesh(new THREE.BoxGeometry(w.size[0], w.size[1], w.size[2]), wallMat);
      wall.position.set(w.pos[0], w.pos[1], w.pos[2]);
      wall.castShadow = true;
      wall.userData = { radius: 0, solid: true, isWall: true };
      g.add(wall);
    });
    return g;
  },

  createBullet() {
    const g = new THREE.Mesh(
      new THREE.SphereGeometry(0.08, 6, 6),
      new THREE.MeshBasicMaterial({ color: 0xffee44 })
    );
    g.userData = { radius: 0.08 };
    return g;
  },

  createGrenade() {
    const g = new THREE.Mesh(
      new THREE.SphereGeometry(0.15, 8, 6),
      new THREE.MeshLambertMaterial({ color: 0x3a4a2a, flatShading: true })
    );
    g.castShadow = true;
    return g;
  }
};