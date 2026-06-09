/*
 * COMMAND13.C:
 *
 *      Additional user routines.
 *
 *      Copyright (C) 1991, 1992, 1993 Brett J. Vickers
 *
 */

#include <stdlib.h>
#include "mstruct.h"
#include "mextern.h"

/**********************************************************************/
/*                              무적수련                              */
/**********************************************************************/

void invince_train_ok();

int invince_train(ply_ptr, cmnd)
creature        *ply_ptr;
cmd             *cmnd;
{
  int fd,new_class=0,i, k=0, l;
  room    *rom_ptr;

  fd=ply_ptr->fd;

  rom_ptr=ply_ptr->parent_rom;

  if(F_ISSET(ply_ptr, PBLIND)){
    ANSI(fd, RED);
    print(fd, "당신은 눈이 멀어 무적수련을 할 수 없습니다!");
    ANSI(fd, WHITE);
    return(0);
  }

  if(!F_ISSET(rom_ptr, RTRAIN)) {
    print(fd, "이 곳은 수련장이 아닙니다!");
    return(0);
  }

  for(i=0; i<3; i++) {
    new_class*=2;
    new_class|=F_ISSET(rom_ptr,RTRAIN+i+1)?1:0;
  }

  for(l=0 ; l<8 ; l++) {
    if(S_ISSET(ply_ptr, l+SASSASSIN))
      k++;
  }
   
  if(k==0) k=1;
   
  if(ply_ptr->class < INVINCIBLE ) {
    print(fd,"무적 이상만 가능합니다.");
    return(0);
  }
  if(S_ISSET(ply_ptr, new_class + 100 )) {
    print(fd, "이미 이 직업의 무적수련을 했습니다.");
    return(0);
  }
  /*  if(!(new_class == 0 || new_class == 1 || new_class == 3 || new_class == 4)) {
      print(fd, "아직은 권법가, 도술사, 자객, 검사만 무적수련 가능합니다.");
      return(0);
      }

  if(ply_ptr->class > INVINCIBLE && ply_ptr->experience<(100000000+1000000*k)) {
    print(fd, "초인이 무적수련을 하려면 경험치 %d만이 필요합니다.", 200*k);
    return(0);
  }
  */
  
  if(ply_ptr->experience<1000000*k) {
    print(fd, "무적수련을 하려면 경험치 %d만이 필요합니다.", 100*k);
    return(0);
  }
  invince_train_ok(fd,1,"");
  return (DOPROMPT);
}

int invince_train_main(ply_ptr)
creature *ply_ptr;
{
  room    *rom_ptr;
  int     fd, i, l, k=0;
  int n,new_class=0;

  fd = ply_ptr->fd;
  rom_ptr = ply_ptr->parent_rom;

  for(i=0; i<3; i++) {
    new_class*=2;
    new_class|=F_ISSET(rom_ptr,RTRAIN+i+1)?1:0;
  }

  for(l=0 ; l<8 ; l++) {
    if(S_ISSET(ply_ptr, l+SASSASSIN)) 
      k++;
  }
  if(k==0) k=1;
  /*
  if(ply_ptr->class > INVINCIBLE)      
    ply_ptr->experience -= 2000000L*k;
  else */
  ply_ptr->experience -= 1000000L*k;

  if(ply_ptr->class == CARETAKER && ply_ptr->experience < 100000000L)
    ply_ptr->class = INVINCIBLE;

  S_SET(ply_ptr, new_class+100);

  if(ply_ptr->class >= INVINCIBLE && ply_ptr->pdice < 5)
    ply_ptr->pdice =(k+1)/2;

  n = exp_to_lev(ply_ptr->experience);
  
  if(ply_ptr->class == INVINCIBLE) {	
    while(ply_ptr->level > n)
      down_level(ply_ptr);
  }
  print(fd, "\n무적수련이 완료되었습니다.");
  broadcast_all("\n### %s님이 %s 무적수련을 완료했습니다.", ply_ptr->name, class_str[new_class+1]);
  
  return(0);
}

void invince_train_ok(fd,param,str)
int fd;
int param;
char *str;
{
int l, k=0;
   
   for(l=0 ; l<8 ; l++) {
    if(S_ISSET(Ply[fd].ply, l+SASSASSIN))
      k++;
}
   if(k==0) k=1;
   
   switch(param) {
  case 1:
    if(Ply[fd].ply->class > INVINCIBLE) {
      print(fd,"초인이 무적수련을 하려면 경험치 %d만이 필요합니다.\n", 200*k);
      print(fd,"무적수련 이후 경험치가 1억이 안되면 무적으로 직업이 바뀝니다.\n");
      print(fd,"무적수련을 하시겠습니까?(예/아니오): ");
    }
    else {
      print(fd,"무적수련을 하려면 경험치 %d만이 필요합니다.\n", 100*k);
      print(fd,"무적수련을 하시겠습니까?(예/아니오): ");
    }
    F_SET(Ply[fd].ply,PREADI);
    output_buf();
    Ply[fd].io->intrpt &= ~1;
    RETURN(fd,invince_train_ok,2);
  case 2:
    F_CLR(Ply[fd].ply,PREADI);
    if(!strncmp(str,"예",2)) {
      invince_train_main(Ply[fd].ply);
    }
    else
      print(fd,"무적수련이 되지 않았습니다");
    RETURN(fd,command,1);
  }
}

/**********************************************************************/
/*                             백보신권                               */
/**********************************************************************/

long ply_invincible_kick_time[PMAX];

int invincible_kick(ply_ptr, cmnd)
creature        *ply_ptr;
cmd             *cmnd;
{
   creature        *crt_ptr;
   room            *rom_ptr;
   long            i, t;
   int             j, k=0, m=0, n, chance, chance2, fd, p;
   
   fd = ply_ptr->fd;
   rom_ptr = ply_ptr->parent_rom;
   
   if(cmnd->num < 2 || F_ISSET(ply_ptr, PBLIND)) {
      print(fd, "누굴 공격합니까?\n");
      return(0);
   }
   
   if(ply_ptr->class < INVINCIBLE) {

	 ANSI(fd, CYAN);
	 print(fd, "무적");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "이상만 쓸수 있는 기술입니다.\n");
	 return(0);

   }
   if(!S_ISSET(ply_ptr, SBARBARIAN)) {

	 print(fd, "아직 ");
	 ANSI(fd, CYAN);
	 print(fd, "권법가");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "를 ");
	 ANSI(fd, CYAN);
	 print(fd, "무적수련");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "하지 않았습니다.\n");
	 return(0);
   }
   
   t = time(0);
   
   if(ply_invincible_kick_time[fd] > t) {
      please_wait(fd, ply_invincible_kick_time[fd] -t);
      return(0);
   }
   
   crt_ptr = find_crt(ply_ptr, rom_ptr->first_mon,
		      cmnd->str[1], cmnd->val[1]);
   
   if(!crt_ptr) {
      cmnd->str[1][0] = up(cmnd->str[1][0]);
      crt_ptr = find_crt(ply_ptr, rom_ptr->first_ply,
			 cmnd->str[1], cmnd->val[1]);
      
      if(!crt_ptr || crt_ptr == ply_ptr) {
	 print(fd, "그런 것은 여기 없습니다.\n");
	 return(0);
      }
      
   }
   
   if(crt_ptr->type == PLAYER) {
      if(!AT_WAR && F_ISSET(rom_ptr, RNOKIL)) {
	 print(fd, "이 방에서는 싸울 수 없습니다.\n");
	 return(0);
      }
      
      if(!F_ISSET(ply_ptr, PFAMIL) || !F_ISSET(crt_ptr, PFAMIL)) {
	 if(!F_ISSET(ply_ptr, PCHAOS) && !F_ISSET(rom_ptr, RSUVIV)) {
	    print(fd, "당신은 선해서 다른 사용자를 공격할 수 없습니다.\n");
	    return (0);
	 }
	 if(!F_ISSET(crt_ptr, PCHAOS) && !F_ISSET(rom_ptr, RSUVIV)) {
	    print(fd, "그 사용자는 선해서 보호받고 있습니다.\n");
	    return (0);
	 }
      }
      else if(check_war(ply_ptr->daily[DL_EXPND].max, crt_ptr->daily[DL_EXPND].max)) {
	 if(!F_ISSET(ply_ptr, PCHAOS) && !F_ISSET(rom_ptr, RSUVIV)) {
	    print(fd, "당신은 선해서 다른 사용자를 공격할 수 없습니다.\n");
	    return (0);
	 }
	 if(!F_ISSET(crt_ptr, PCHAOS) && !F_ISSET(rom_ptr, RSUVIV)) {
	    print(fd, "그 사용자는 선해서 보호받고 있습니다.\n");
	    return (0);
	 }
      }
      if(is_charm_crt(ply_ptr->name, crt_ptr)&& F_ISSET(crt_ptr, PCHARM)) {
	 print(fd, "당신은 %S%j 너무 좋아해 그렇게 할 수 없습니다.\n", crt_ptr->name,"3");
	 return(0);
      }
      
   }

   ply_ptr->lasttime[LT_ATTCK].ltime = t;
   
   F_CLR(ply_ptr, PHIDDN);
   if(F_ISSET(ply_ptr, PINVIS)) {
      F_CLR(ply_ptr, PINVIS);
      print(fd, "당신의 모습이 서서히 드러납니다.\n");
      broadcast_rom(fd, ply_ptr->rom_num, "\n%M의 모습이 서서히 드러납니다.",
		    ply_ptr);
   }
   
   if(crt_ptr->type != PLAYER) {
      if(F_ISSET(crt_ptr, MUNKIL)) {
	 print(fd, "당신은 %s를 해칠 수 없습니다.\n",
	       F_ISSET(crt_ptr, MMALES) ? "그":"그녀");
	 return(0);
      }
      if(mrand(0,1) && (ply_ptr->piety < crt_ptr->piety) && F_ISSET(crt_ptr, MMGONL)) {
		  print(fd, "당신의 공격이 %M에게 아무소용이 없는듯 합니다.\n", crt_ptr);
	 return(0);
      }
      if(mrand(0,1) && F_ISSET(crt_ptr, MENONL)) {
	 if(!ply_ptr->ready[WIELD-1] ||
	    ply_ptr->ready[WIELD-1]->adjustment < 1) {
	    print(fd, "당신의 공격이 %M에게 아무 소용이 없는듯 합니다.\n", crt_ptr);
	    return(0);
	 }
      }
      add_enm_crt(ply_ptr->name, crt_ptr);
   }

   print(fd, "\n천하권법의 최고봉이 있으니 백보신권이라 하느니..\n");
   print(fd, "백장 밖의 적도 살상할 수 있으니 세상 어느 누가 두려워 하지 않으리요~~~\n");
   
chance = 50 + (((ply_ptr->level+3)/4) - ((crt_ptr->level+3)/4))*2 + bonus[ply_ptr->intelligence]*3 + bonus[ply_ptr->dexterity]*3;
   chance = MIN(90, chance);
   if(F_ISSET(ply_ptr, PBLIND))
     chance = MIN(20, chance);
   
   if(mrand(1,100) <= chance) {
      
      n = ply_ptr->thaco - crt_ptr->armor/10;
      if(mrand(1,20) >= n) {
	 chance2 = (20-ply_ptr->thaco)/9 + mrand(1,((ply_ptr->level+23)/30));
	 print(fd, "당신의 주먹에 기가 모이며 백장 밖의 적에게 타격을 입힙니다.\n\n");
	 ply_invincible_kick_time[fd] = t+ (20-ply_ptr->dexterity/7);  
	 for(j=0; j < chance2; j++){ 
	    n = mdice(ply_ptr) * 3 + mrand(0,ply_ptr->strength)*2 ;
	    m = MIN(crt_ptr->hpcur, n);     
	    
	    /* crt_ptr->hpcur -= n; */
	    
		 print(fd, "당신은 ");
		 ANSI(fd, YELLOW);
		 print(fd, "백보신권 ");
		 ANSI(fd, WHITE);
		 ANSI(fd, NORMAL);
		 print(fd, "으로 ");
		 ANSI(fd, GREEN);
		 print(fd, "%d", m);
		 ANSI(fd, WHITE);
		 ANSI(fd, NORMAL);
		 print(fd, "점의 공격을 가했습니다.\n");

/*	    print(fd, "당신은 백보신권으로 %d점의 공격을 가했습니다.\n\n", m);   */

	    broadcast_rom2(fd, crt_ptr->fd, crt_ptr->rom_num,
			   "\n\n%M이 %M에게 백보신권으로 %d점의 공격을 가합니다.", ply_ptr, crt_ptr, m);
	    print(crt_ptr->fd, "%M이 백보신권으로 %d점의 공격을 가했습니다.\n\n", ply_ptr, m);
	    crt_ptr->hpcur -= m;
	    k += m;
	    if(crt_ptr->hpcur < 1) break;
	 }
	 if(crt_ptr->type != PLAYER) {
                                add_enm_dmg(ply_ptr->name, crt_ptr, k);
	 }
         print(fd, "당신은 총");
         ANSI(fd, GREEN);
         print(fd," %d 연타",j);
         ANSI(fd, YELLOW);
         print(fd, " %d점",k);
         ANSI(fd, WHITE);
         print(fd, "의 공격을");
         ANSI(fd, CYAN);
         print(fd, " %M",crt_ptr);
         ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);	
	 print(fd, "에게 가했습니다.\n");

	 if(crt_ptr->hpcur < 1) {
	    print(fd, "당신은 %M%j 죽였습니다.\n", crt_ptr,"3");
	    broadcast_rom2(fd, crt_ptr->fd,
			   ply_ptr->rom_num,
                                               "\n%M%j %M%j 죽였습니다.", ply_ptr,"1",
			   crt_ptr,"3");
	    
	    die(crt_ptr, ply_ptr);
	 }
	 else
	   check_for_flee(ply_ptr, crt_ptr);
      }
      else {
	 print(fd, "당신의 ");
	 ANSI(fd, YELLOW);
	 print(fd, "백보신권");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "이 실패했습니다.\n");

	 print(crt_ptr->fd, "\n%M이 당신에게 백보신권을 구사하려고 합니다.\n", ply_ptr);
	 broadcast_rom2(fd, crt_ptr->fd, crt_ptr->rom_num,
			"\n%M이 %M에게 백보신권으로 공격하려고 합니다.", ply_ptr,
			crt_ptr);
	 ply_invincible_kick_time[fd] = t+(15-ply_ptr->dexterity/6);
      }
   }
   
   else {

	 print(fd, "당신의 ");
	 ANSI(fd, YELLOW);
	 print(fd, "백보신권");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "이 실패했습니다.\n");

      print(crt_ptr->fd, "%M이 당신에게 백보신권을 구사하려고 합니다.\n", ply_ptr);
      broadcast_rom2(fd, crt_ptr->fd, crt_ptr->rom_num,
		     "\n%M이 %M에게 백보신권으로 공격하려고 합니다.", ply_ptr, crt_ptr);
      ply_invincible_kick_time[fd] = t+ (15 -MIN(7, ply_ptr->dexterity/3));
   }
   return(0);

}

/**********************************************************************/
/*                             일격필살                               */
/**********************************************************************/


long ply_one_kill_time[PMAX];

int one_kill(ply_ptr, cmnd)
creature        *ply_ptr;
cmd             *cmnd;
{
   creature *crt_ptr;
   room           *rom_ptr;
   long            i, t;
   int              chance, m, dur, dmg, fd, p, addprof;
   fd = ply_ptr->fd;
   rom_ptr = ply_ptr->parent_rom;
   if(cmnd->num < 2) {
      print(fd,
	    "\n누구를 공격합니까?\n");
      return(0);
   }
   if(ply_ptr->class < INVINCIBLE) {
	 ANSI(fd, CYAN);
	 print(fd, "무적");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "이상만 쓸수 있는 기술입니다.\n");
	 return(0);
   }
   if(!S_ISSET(ply_ptr, SASSASSIN)) {

	 print(fd, "아직 ");
	 ANSI(fd, CYAN);
	 print(fd, "자객");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "을 ");
	 ANSI(fd, CYAN);
	 print(fd, "무적수련");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "하지 않았습니다.\n");
	 return(0);

   }

   
   crt_ptr = find_crt(ply_ptr, rom_ptr->first_mon, cmnd->str[1], cmnd->val[1]);
   if(!crt_ptr) {
      print(fd, "\n그런 것은 존재하지 않습니다.\n");
      return(0);
   }
   if(crt_ptr->type != PLAYER && is_enm_crt(ply_ptr->name, crt_ptr)) {
     print(fd, "당신은 %s와 싸우는 중입니다.\n",
	   F_ISSET(crt_ptr, MMALES) ? "그":"그녀");
     return(0);
   }

   t = time(0);
   
   if(ply_one_kill_time[fd] > t) {
      please_wait(fd, ply_one_kill_time[fd] -t);
      return(0);
   }

   if(F_ISSET(ply_ptr, PINVIS)) {
      F_CLR(ply_ptr, PINVIS);
      print(fd, "\n당신의 모습이 나타나기 시작합니다.\n");
      broadcast_rom(fd, ply_ptr->rom_num, "%M의 모습이 보이기 시작합니다.", ply_ptr);
   }

   if(crt_ptr->type == PLAYER) {
      if(!AT_WAR && F_ISSET(rom_ptr, RNOKIL)) {
	 print(fd, "이 방에서는 싸울 수 없습니다.\n");
	 return(0);
      }
      
      if(!F_ISSET(ply_ptr, PFAMIL) || !F_ISSET(crt_ptr, PFAMIL)) {
	 if(!F_ISSET(ply_ptr, PCHAOS) && !F_ISSET(rom_ptr, RSUVIV)) {
	    print(fd, "당신은 선해서 다른 사용자를 공격할 수 없습니다.\n");
	    return (0);
	 }
	 if(!F_ISSET(crt_ptr, PCHAOS) && !F_ISSET(rom_ptr, RSUVIV)) {
	    print(fd, "그 사용자는 선해서 보호받고 있습니다.\n");
	    return (0);
	 }
      }
      else if(check_war(ply_ptr->daily[DL_EXPND].max, crt_ptr->daily[DL_EXPND].max)) {
	 if(!F_ISSET(ply_ptr, PCHAOS) && !F_ISSET(rom_ptr, RSUVIV)) {
	    print(fd, "당신은 선해서 다른 사용자를 공격할 수 없습니다.\n");
	    return (0);
	 }
	 if(!F_ISSET(crt_ptr, PCHAOS) && !F_ISSET(rom_ptr, RSUVIV)) {
	    print(fd, "그 사용자는 선해서 보호받고 있습니다.\n");
	    return (0);
	 }
      }
      if(is_charm_crt(ply_ptr->name, crt_ptr)&& F_ISSET(crt_ptr, PCHARM)) {
	 print(fd, "당신은 %S%j 너무 좋아해 그렇게 할 수 없습니다.\n", crt_ptr->name,"3");
	 return(0);
      }
      
   }
   if(!ply_ptr->ready[WIELD-1] || (ply_ptr->ready[WIELD-1]->type
				   != SHARP && ply_ptr->ready[WIELD-1]->type != THRUST)) {
      print(fd, "일격필살을 하시려면 도나 검종류의 무기가 필요합니다.");
      return(0);
   }
   
   ply_ptr->lasttime[LT_ATTCK].ltime = t;

   F_CLR(ply_ptr, PHIDDN);
   if(F_ISSET(ply_ptr, PINVIS)) {
      F_CLR(ply_ptr, PINVIS);
      print(fd, "당신의 모습이 서서히 드러납니다.\n");
      broadcast_rom(fd, ply_ptr->rom_num, "\n%M의 모습이 서서히 드러납니다.",
		    ply_ptr);
   }
   
   if(crt_ptr->type != PLAYER) {
      if(F_ISSET(crt_ptr, MUNKIL)) {
	 print(fd, "당신은 %s를 해칠 수 없습니다.\n", F_ISSET(crt_ptr, MMALES) ? "그":"그녀");
	 return(0);
      }
      ply_one_kill_time[fd] =t+5;
      ply_ptr->lasttime[LT_TURNS].interval = 15L;
      if(mrand(0,1) && (ply_ptr->piety < crt_ptr->piety) && F_ISSET(crt_ptr, MMGONL)) {
	 print(fd, "당신의 공격이 %M에게 아무소용이 없는듯 합니다.\n", crt_ptr);
	 return(0);
      }
      if(mrand(0,1) && F_ISSET(crt_ptr, MENONL)) {
	 if(!ply_ptr->ready[WIELD-1] || ply_ptr->ready[WIELD-1]->adjustment < 1) {
	    print(fd, "당신의 공격이 %M에게 아무 소용이 없는듯 합니다.\n", crt_ptr);
	    return(0);
	 }
      }
      add_enm_crt(ply_ptr->name, crt_ptr);
   }

   print(fd,"\n일천년 자객역사의 살신지왕 무풍 왈~~~ \n");
   print(fd,"\"자객의 도는 적을 일격에 살생하는 것이니\n실패는 곧 죽음을 부르리라 !!!\"\n");
   print(fd,"당신은 혼신의 힘을 모아 적의 미심혈을 찌릅니다.\n\n");
   chance = (ply_ptr->dexterity - crt_ptr->dexterity)*3+ (20 - ply_ptr->thaco)*2 + mdice(ply_ptr->ready[WIELD-1])*2;
   chance = MIN(chance, 70);

   if (mrand(1,100) <= chance) {

     if (ply_ptr->ready[WIELD-1]->shotscur > 0)
       ply_ptr->ready[WIELD-1]->shotscur--;
     
     if (ply_ptr->ready[WIELD-1]) {
       if(ply_ptr->ready[WIELD-1]->shotscur < 1) {
	 print(fd, "\n%S%j 부서져 버렸습니다.\n", ply_ptr->ready[WIELD-1]->name,"1");
	 add_obj_crt(ply_ptr->ready[WIELD-1], ply_ptr);
	 ply_ptr->ready[WIELD-1] = 0;
	 return(0);
       }
     }

     if(ply_ptr->ready[WIELD-1]) {
	p = MIN(ply_ptr->ready[WIELD-1]->type, MISSILE);
	addprof = (m * crt_ptr->experience) / crt_ptr->hpmax;
	addprof = MIN(addprof, crt_ptr->experience);
	ply_ptr->proficiency[p] += addprof;
     }
     
     if (ply_ptr->class >= BULSA) 
     	dmg = (crt_ptr->hpcur/2) + ply_ptr->dexterity*3 + mdice(ply_ptr->ready[WIELD-1])*mrand(5,7);
     else if (ply_ptr->class == CARETAKER)
     	dmg = (crt_ptr->hpcur/2) + ply_ptr->dexterity*3 + mdice(ply_ptr->ready[WIELD-1])*mrand(3,7);
     else 
     	dmg = (crt_ptr->hpcur/2) + ply_ptr->dexterity*3 + mdice(ply_ptr->ready[WIELD-1])*mrand(1,7);
     
     /* Garbagge Code 
     if (dmg < 0) {
		dmg = (crt_ptr->hpcur/2) * 1 + 100;
     }
     */
     m = MIN(crt_ptr->hpcur, dmg);
     crt_ptr->hpcur -= m;

     print(fd, "당신은 ");
     ANSI(fd, YELLOW);
     print(fd, "일격필살");
     ANSI(fd, WHITE);
     ANSI(fd, NORMAL);
     print(fd, "로 %M의 급소를 찔러" , crt_ptr);
     ANSI(fd, GREEN);
     print(fd, "%d", m);
     ANSI(fd, WHITE);
     ANSI(fd, NORMAL);
     print(fd, "점의 피해를 입혔습니다.\n");

     display_status(fd, crt_ptr);   /* 몹 상태 나타내는 부분 */
     print(fd, "\n");
      
     broadcast_rom(fd, ply_ptr->rom_num, "%M이 일격필살로 %m의 급소를 찔러서 %d의 피해를 입혔습니다.\n", ply_ptr, crt_ptr, m);
     if(crt_ptr->type != PLAYER) {
	 add_enm_dmg(ply_ptr->name, crt_ptr, m);
     }

     if(crt_ptr->hpcur < 1) {
	 print(fd, "\n당신은 일격필살로 %M%j 죽였습니다.", crt_ptr,"3");
	 broadcast_rom(fd, ply_ptr->rom_num, "\n%M%j 일격필살로 %M%j 죽였습니다.", ply_ptr,"1", crt_ptr,"3");
	 die(crt_ptr, ply_ptr);
     }
     else
	check_for_flee(ply_ptr, crt_ptr);
   }
   else {
      print(fd,"\n%M의 일격필살을 %M%j 미리 알아차렸습니다.\n\n", ply_ptr, crt_ptr,"1");
      if((100-ply_ptr->armor) < 200 || ply_ptr->hpcur < (ply_ptr->hpmax/3)*2 ) {
	 print(fd,"\n%M%j 역공을 가해 %M에게 %d의 치명타를 가합니다.\n", crt_ptr,"1", ply_ptr, (ply_ptr->hpcur/3)*2);
	 ply_ptr->hpcur -= (ply_ptr->hpcur/3)*2;
      }
   }
   ply_one_kill_time[fd] = t + 10;
   return(0);
}

/**********************************************************************/
/*                             영자팔법                               */
/**********************************************************************/

long ply_eight_time[PMAX];

int eight(ply_ptr, cmnd)
creature        *ply_ptr;
cmd            *cmnd;
{
   ctag            *cp;
   room            *rom_ptr;
   long            i, t;
   int              chance, j, k, m, dmg, fd, enm_thaco=0, m2=0, dur, p, addprof;
   fd = ply_ptr->fd;
   
   rom_ptr = ply_ptr->parent_rom;
  
   if(ply_ptr->class < INVINCIBLE) {
	 ANSI(fd, CYAN);
	 print(fd, "무적");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "이상만 쓸수 있는 기술입니다.\n");
	 return(0);
   }
   if(!S_ISSET(ply_ptr, SFIGHTER)) {

	 print(fd, "아직 ");
	 ANSI(fd, CYAN);
	 print(fd, "검사");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "를 ");
	 ANSI(fd, CYAN);
	 print(fd, "무적수련");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "하지 않았습니다.\n");
	 return(0);

   }
   
   t = time(0);
   
   if(ply_eight_time[fd] > t) {
     please_wait(fd, ply_eight_time[fd] -t);
     return(0);
   }

   if(!ply_ptr->ready[WIELD-1] || (ply_ptr->ready[WIELD-1]->type != THRUST )) {
     print(fd, "영자팔법을 구사하시려면  검종류의 무기가 필요합니다.");
     return(0);
   }
   cp = rom_ptr->first_mon;
   while(cp) {
     
     if((F_ISSET(ply_ptr, PDINVI) ? 1:!F_ISSET(cp->crt, MINVIS)) && !F_ISSET(cp->crt, MHIDDN) \
	&& !F_ISSET(cp->crt, MUNKIL )) {
       
       m2++;
       enm_thaco += (20 - cp->crt->thaco);
       add_enm_crt(ply_ptr->name, cp->crt);
       }
     cp = cp->next_tag;
   }


   if(m2<1) {
     print(fd,"이 방에는 당신이 공격할 적이 없습니다.");
     return(0);
   }

   ply_ptr->lasttime[LT_ATTCK].ltime = t;
   
   F_CLR(ply_ptr, PHIDDN);
   if(F_ISSET(ply_ptr, PINVIS)) {
      F_CLR(ply_ptr, PINVIS);
      print(fd, "당신의 모습이 서서히 드러납니다.\n");
      broadcast_rom(fd, ply_ptr->rom_num, "\n%M의 모습이 서서히 드러납니다.",
		    ply_ptr);
   }

   print(fd,"검의 모든 방위는 여기서 나오니 이것이 바로 영자팔법이라~ \n");
   print(fd,"\"세상의 어느 누가 영자팔법의 방위를 피하랴! ");
   print(fd,"    이야압~~!!!\" \n\n당신은 영자팔법으로 팔방의 모든 방위를 차단하며 공격을 합니다.\n\n");
   chance = (20- ply_ptr->thaco) + 2*bonus[ply_ptr->dexterity] -  enm_thaco / m2 + (ply_ptr->level+29)/30 ;
   chance = MIN(chance, 20);
   if(chance < 5) chance = 5;

   if (mrand(1,22) <= chance) {

     if(ply_ptr->ready[WIELD-1]->shotscur > 0)
       ply_ptr->ready[WIELD-1]->shotscur--;
     
     if(ply_ptr->ready[WIELD-1]) {
       if(ply_ptr->ready[WIELD-1]->shotscur < 1) {
	 print(fd, "\n%S%j 부서져 버렸습니다.\n",
	       ply_ptr->ready[WIELD-1]->name,"1");
	 add_obj_crt(ply_ptr->ready[WIELD-1], ply_ptr);
	 ply_ptr->ready[WIELD-1] = 0;
	 return(0);
       }
     }
     
     k = MIN((chance+1)/3, m2);
     cp = rom_ptr->first_mon;
     for( j=0 ;j<k;j++)  {
       if((F_ISSET(ply_ptr, PDINVI) ? 1:!F_ISSET(cp->crt, MINVIS)) && !F_ISSET(cp->crt, MHIDDN) \
	  && !F_ISSET(cp->crt, MUNKIL )) {
	 dmg = mdice(ply_ptr)*3 + mdice(ply_ptr->ready[WIELD-1])*4;
	 dmg = MIN(cp->crt->hpcur, dmg);
	 
	 if(ply_ptr->ready[WIELD-1]) {
	   p = MIN(ply_ptr->ready[WIELD-1]->type, MISSILE);
	   addprof = (dmg * cp->crt->experience) / (cp->crt->hpmax * k);
	   addprof = MIN(addprof, cp->crt->experience);
	   ply_ptr->proficiency[p] += addprof;
	 }

	 if(mrand(0,1) && (ply_ptr->piety < cp->crt->piety) && F_ISSET(cp->crt, MMGONL)) {
	   print(fd, "당신의 검이 %M에게 아무런 상처도 내지 못합니다.\n", cp->crt);
	   dmg = 1;
	 }
	 if(F_ISSET(cp->crt, MENONL) && ply_ptr->ready[WIELD-1]->adjustment < 1) {
	   print(fd, "당신의 검이 %M의 갑옷을 뚫기엔 역부족입니다.\n", cp->crt);
	   dmg = 1;
	 }
	 add_enm_dmg(ply_ptr->name, cp->crt , dmg);
	 dur = MIN(20, dmg/20);
	 
	 cp->crt->lasttime[LT_ATTCK].ltime = time(0);
	 cp->crt->lasttime[LT_ATTCK].interval = dur;
	 

		 print(fd, "당신은 ");
		 ANSI(fd, YELLOW);
		 print(fd, "영자팔법");
		 ANSI(fd, WHITE);
		 ANSI(fd, NORMAL);
		 print(fd, "으로 %m에게 " , cp->crt);
		 ANSI(fd, GREEN);
		 print(fd, "%d", dmg);
		 ANSI(fd, WHITE);
		 ANSI(fd, NORMAL);
		 print(fd, "의 피해를 입힙니다.\n");

/*	 print(fd, "\n당신은 영자팔법으로 %m에게 %d의 피해를 입힙니다.\n", cp->crt, dmg);     */
	 broadcast_rom(fd, ply_ptr->rom_num,
		       "\n%M이 영자팔법으로 %m에게 %d의 피해를 입혔습니다.\n", \
		       ply_ptr,cp->crt , dmg);
	 if(dur > 5) {
	   cp->crt->lasttime[LT_ATTCK].interval = dur;
	   print(fd, "당신에게 입은 상처로 %m%j 움직이지 못합니다.\n", cp->crt, "1");
	 }
	 cp->crt->hpcur -= dmg;
       }	 
       if(cp->crt->hpcur < 1) {
	 print(fd, "\n당신의 뛰어난 영자팔법으로 %M%j 죽였습니다.", cp->crt ,"3");
	 broadcast_rom(fd, ply_ptr->rom_num,
		       "\n%M%j 영자팔법으로 %M%j 죽였습니다.", ply_ptr,"1", cp->crt,"3");
	 die(cp->crt, ply_ptr);
	 cp = rom_ptr->first_mon;
       }
       else {
	 check_for_flee(ply_ptr, cp->crt);
	 cp = cp->next_tag;
       }
     }
   }
     
     
   else {
	 print(fd, "당신의 ");
	 ANSI(fd, YELLOW);
	 print(fd, "영자팔법");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "이 적의 기세에 눌려 실패했습니다.\n");

     broadcast_rom(fd, ply_ptr->rom_num,
		   "\n%M의 영자팔법이 적의 기세에 눌려 실패했습니다.\n", ply_ptr);
     
     if( !F_ISSET(ply_ptr->ready[WIELD-1], ONSHAT) && \
	 ply_ptr->ready[WIELD-1]->shotscur < ply_ptr->ready[WIELD-1]->shotsmax/2 && ((mrand(1,5) <= 2))) {
       print(fd, "\n%S%j 산산히 부서집니다.",
	     ply_ptr->ready[WIELD-1]->name,"1");
       broadcast_rom(fd, ply_ptr->rom_num,
		     "\n%s의 %S%j 산산히 부서집니다.",
		     F_ISSET(ply_ptr, PMALES) ? "그":"그녀",
		     ply_ptr->ready[WIELD-1]->name,"1");
       free_obj(ply_ptr->ready[WIELD-1]);
       ply_ptr->ready[WIELD-1] = 0;
     }
     
   }
   ply_eight_time[fd] = t+ (15 - MIN(7, ply_ptr->dexterity/4));
   return(0);
}

/**********************************************************************/
/*                            나한진                                  */
/**********************************************************************/

long ply_nahan_time[PMAX];

int nahan(ply_ptr, cmnd)
creature        *ply_ptr;
cmd            *cmnd;
{
   ctag            *cp;
   creature        *crt_ptr;
   room            *rom_ptr;
   long            i, t;
   int              chance, dmg, fd, m2=0, ply_mp=0;
   fd = ply_ptr->fd;
   
   rom_ptr = ply_ptr->parent_rom;
  
   if(cmnd->num < 2) {
      print(fd,
	    "\n누구를 공격합니까?\n");
      return(0);
   }

   if(ply_ptr->class < INVINCIBLE) {
	 ANSI(fd, CYAN);
	 print(fd, "무적");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "이상만 쓸수 있는 기술입니다.\n");
	 return(0);
   }
   if(!S_ISSET(ply_ptr, SCLERIC)) {

	 print(fd, "아직 ");
	 ANSI(fd, CYAN);
	 print(fd, "불제자");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "를 ");
	 ANSI(fd, CYAN);
	 print(fd, "무적수련");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "하지 않았습니다.\n");
	 return(0);

   }

   crt_ptr = find_crt(ply_ptr, rom_ptr->first_mon, cmnd->str[1], cmnd->val[1]);
   if(!crt_ptr) {
      print(fd, "\n그런 것은 존재하지 않습니다.\n");
      return(0);
   }

   t = time(0);
   
   if(ply_nahan_time[fd] > t) {
     please_wait(fd, ply_nahan_time[fd] -t);
     return(0);
   }

   ply_ptr->lasttime[LT_ATTCK].ltime = t;
   
   F_CLR(ply_ptr, PHIDDN);
   if(F_ISSET(ply_ptr, PINVIS)) {
      F_CLR(ply_ptr, PINVIS);
      print(fd, "당신의 모습이 서서히 드러납니다.\n");
      broadcast_rom(fd, ply_ptr->rom_num, "\n%M의 모습이 서서히 드러납니다.",
		    ply_ptr);
   }

   if(crt_ptr->type != PLAYER) {
      if(F_ISSET(crt_ptr, MUNKIL)) {
	 print(fd, "당신은 %s를 해칠 수 없습니다.\n",
	       F_ISSET(crt_ptr, MMALES) ? "그":"그녀");
	 return(0);
      }
      if(mrand(0,1) && (ply_ptr->piety < crt_ptr->piety) && F_ISSET(crt_ptr, MMGONL)) {
	 print(fd, "당신의 공격이 %M에게 아무소용이 없는듯 합니다.\n", crt_ptr);
	 return(0);
      }
      add_enm_crt(ply_ptr->name, crt_ptr);
   }

   if(crt_ptr->type == PLAYER) {
     if(!AT_WAR && F_ISSET(rom_ptr, RNOKIL)) {
       print(fd, "이 방에서는 싸울 수 없습니다.\n");
       return(0);
     }
     
     if(!F_ISSET(ply_ptr, PFAMIL) || !F_ISSET(crt_ptr, PFAMIL)) {
       if(!F_ISSET(ply_ptr, PCHAOS) && !F_ISSET(rom_ptr, RSUVIV)) {
	 print(fd, "당신은 선해서 다른 사용자를 공격할 수 없습니다.\n");
	 return (0);
       }
       if(!F_ISSET(crt_ptr, PCHAOS) && !F_ISSET(rom_ptr, RSUVIV)) {
	 print(fd, "그 사용자는 선해서 보호받고 있습니다.\n");
	 return (0);
       }
     }
     else if(check_war(ply_ptr->daily[DL_EXPND].max, crt_ptr->daily[DL_EXPND].max)) {
       if(!F_ISSET(ply_ptr, PCHAOS) && !F_ISSET(rom_ptr, RSUVIV)) {
	 print(fd, "당신은 선해서 다른 사용자를 공격할 수 없습니다.\n");
	 return (0);
       }
       if(!F_ISSET(crt_ptr, PCHAOS) && !F_ISSET(rom_ptr, RSUVIV)) {
	 print(fd, "그 사용자는 선해서 보호받고 있습니다.\n");
	 return (0);
       }
     }
     if(is_charm_crt(ply_ptr->name, crt_ptr)&& F_ISSET(crt_ptr, PCHARM)) {
       print(fd, "당신은 %S%j 너무 좋아해 그렇게 할 수 없습니다.\n", crt_ptr->name,"3");
       return(0);
     }
     
   }
   
   cp = rom_ptr->first_ply;
   while(cp) {
     if((F_ISSET(ply_ptr, PDINVI) ? 1:!F_ISSET(cp->crt, MINVIS)) && !(ply_ptr->name == cp->crt->name)) {
       m2++;
       add_enm_crt(cp->crt->name, crt_ptr);
       if(cp->crt->class > INVINCIBLE)
	 ply_mp += MIN(1000, cp->crt->mpcur/2);
       else if(cp->crt->class == INVINCIBLE)
	 ply_mp +=  cp->crt->mpcur/2;
       else 
	 ply_mp +=  cp->crt->mpcur;
     }
     cp = cp->next_tag;
   }
   m2++;
   ply_mp += ply_ptr->mpcur;

   if(m2 < 2) {
     print(fd,"당신 혼자서는 나한진을 펼칠 수 없습니다.\n");
     return(0);
   }
   if(ply_mp < MIN(2000, crt_ptr->mpcur)/m2) {
     print(fd,"당신과 함께 있는 동료들의 도력이 부족합니다.\n");
     return(0);
   }

   print(fd,"만다라의 힘을 빌어 세상의 모든 마룰 가둘 수 있으니 이것이 나한진이라~~ \n");
   print(fd,"\"아미타불~~ 세상의 모든 마를 소멸 시키리라~~!!\"\n\n");
   print(fd,"당신은 당신의 동료들과 함께 나한진을 펴며 적을 공격합니다.\n\n");
   chance = (20- ply_ptr->thaco) - (20-crt_ptr->thaco) + (ply_ptr->level+29)/30 \
     + bonus[ply_ptr->piety]*2 + bonus[ply_ptr->intelligence] ;
   chance = MIN(chance, 20);
   if(chance < 3) chance = 3;
   if (mrand(1,22) <= chance) {
     dmg = (ply_mp/10)*m2 + (mrand(1,ply_ptr->piety) + mrand(1, ply_ptr->intelligence))*m2;
     if(ply_ptr->class > INVINCIBLE) dmg /=2;
     dmg = MIN(crt_ptr->hpcur, dmg);
     crt_ptr->hpcur -= dmg;
     
		 print(fd, "당신은 ");
		 ANSI(fd, YELLOW);
		 print(fd, "나한진 ");
		 ANSI(fd, WHITE);
		 ANSI(fd, NORMAL);
		 print(fd, "을 펼쳐 %M에게 " , crt_ptr);
		 ANSI(fd, GREEN);
		 print(fd, "%d", dmg);
		 ANSI(fd, WHITE);
		 ANSI(fd, NORMAL);
		 print(fd, "점의 피해를 입혔습니다.\n");

/*     print(fd,"당신은 나한진을 펼쳐 %M에게 %d의 피해를 입혔습니다.\n",  crt_ptr, dmg );   */
     
     broadcast_rom(fd, ply_ptr->rom_num,
		   "\n%M이 동료들과 함께 나한진을 펼쳐 %m에게 %d의 피해를 입혔습니다.\n", ply_ptr, crt_ptr, dmg);

     if(crt_ptr->type != PLAYER) {
       add_enm_dmg(ply_ptr->name, crt_ptr, dmg/2);
       
       cp = rom_ptr->first_ply;
       while(cp) {
	 if((F_ISSET(ply_ptr, PDINVI) ? 1:!F_ISSET(cp->crt, MINVIS)) && !(ply_ptr->name == cp->crt->name)) {
	   add_enm_dmg(cp->crt->name, crt_ptr, (dmg/2)/(m2-1));
	   print(cp->crt->fd,"\n당신은 %M의 나한진에 가세하여 %m에게 %d의 피해를 입혔습니다.",ply_ptr, \
		 crt_ptr, (dmg/2)/(m2-1)); 
	 }
	 cp = cp->next_tag;
       }
     }
     
     if(crt_ptr->hpcur < 1) {
       print(fd, "\n당신은 나한진으로 %m%j 죽였습니다.", crt_ptr,"3");
       broadcast_rom(fd, ply_ptr->rom_num,
		     "\n%M의 나한진으로 %m%j 죽였습니다.", ply_ptr, crt_ptr,"3");
       die(crt_ptr, ply_ptr);
     }
     else
       check_for_flee(ply_ptr, crt_ptr);
   }
     
   else {
	 print(fd, "당신과 동료들의 ");
	 ANSI(fd, YELLOW);
	 print(fd, "나한진");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "이 실패했습니다.\n");
     broadcast_rom(fd, ply_ptr->rom_num,
		   "\n%M과 동료들의 나한진이 실패했습니다.\n", ply_ptr);
     
     ply_ptr->mpcur = 0;
   }
   ply_nahan_time[fd] = t+ 25 - MIN(15, (ply_ptr->piety/5 + ply_ptr->intelligence/3));
   return(0);
}


/*++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++*/
/*                  혈마안                                      */
/*++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++*/

long ply_red_eye_time[PMAX];

int red_eye(ply_ptr, cmnd)
creature    *ply_ptr;
cmd     *cmnd;
{
  creature    *crt_ptr;
  creature    *enm_ptr;
  room        *rom_ptr;
  room        *new_rom;
  room        *rom_ply;
  int     fd, t, chance, chance1, dmg;
  ctag	      *cp, *cp_crt; /* 그룹운 찾기 */
  int 	      grp_count = 0, grp_count_crt = 0; /* 그룹원 카운트 */
  
  fd = ply_ptr->fd;
  rom_ptr = ply_ptr->parent_rom;
  
  if(fd < 0) return(0);
  
  if(ply_ptr->class < INVINCIBLE) {
	 ANSI(fd, CYAN);
	 print(fd, "무적");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "이상만 쓸수 있는 기술입니다.\n");
	 return(0);
  }

  if(!S_ISSET(ply_ptr, SPALADIN)) {

	 print(fd, "아직 ");
	 ANSI(fd, CYAN);
	 print(fd, "무사");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "을 ");
	 ANSI(fd, CYAN);
	 print(fd, "무적수련");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "하지 않았습니다.\n");
	 return(0);
  }
  
  if(cmnd->num < 3) {
    print(fd, "\n사용법 : 누구 몹이름 혈마안\n");
    return(0);
  }
  
  /*  lowercize(cmnd->str[2], 1); */

  t = time(0);
   
  if(ply_red_eye_time[fd] > t) {
    please_wait(fd, ply_red_eye_time[fd] -t);
    return(0);
  }

  crt_ptr = find_who(cmnd->str[1]);
 
  if(!crt_ptr || F_ISSET(crt_ptr, PDMINV) ||
     (F_ISSET(crt_ptr, PINVIS) && !F_ISSET(ply_ptr, PDINVI))) {
    print(fd, "\n그런 사람은 존재하지 않습니다.\n");
    return(0);
  }

  /* 그룹원들에게 혈마 금지 */
  if (ply_ptr->following) {
      	cp = ply_ptr->following->first_fol;
  }
  else {
      	cp = ply_ptr->first_fol;
  }
  while (cp && grp_count < 1) {
      	grp_count++;
	cp = cp->next_tag;
  }
  if (grp_count > 0) {
      	print(fd, "그룹원들에게는 혈마를 할 수 없습니다.\n");
	return (0);
  }

  /* 혈마해줄 사람 그룹원 찾기 */
  if (crt_ptr->following) {
      	cp_crt = crt_ptr->following->first_fol;
  }
  else {
        cp_crt = crt_ptr->first_fol;
  }
  while (cp_crt && grp_count_crt < 1) {
      	grp_count_crt++;
	cp_crt=cp_crt->next_tag;
  }
  if (grp_count_crt > 0) {
      	print(fd, "상대방이 그룹이 있네요. 혈마를 할 수 없어요!\n");
	return (0);
  }
  
  
  F_CLR(ply_ptr, PHIDDN);
  if(F_ISSET(ply_ptr, PINVIS)) {
    F_CLR(ply_ptr, PINVIS);
    print(fd, "당신의 모습이 서서히 드러납니다.\n");
    broadcast_rom(fd, ply_ptr->rom_num, "\n%M의 모습이 서서히 드러납니다.",
		  ply_ptr);
  }

  ply_ptr->lasttime[LT_ATTCK].ltime = t;

   chance1 = 50 + ((ply_ptr->level+3)/4) + mrand(1, ply_ptr->intelligence); 
   chance1 = MIN(85,chance);
   if(crt_ptr->class == DM && ply_ptr->class != DM && chance1 < 80) {
     print(fd,"\n%M에게 정신을 연결하지 못했습니다.");
     return(0);
   }
   new_rom = crt_ptr->parent_rom;

   broadcast_rom(fd, ply_ptr->rom_num, 
		  "\n%M이 혈마안을 구사하기 위해 정신을 집중합니다.\n", ply_ptr);

   enm_ptr = find_crt(ply_ptr, new_rom->first_mon, cmnd->str[2], cmnd->val[2]);
   if(!enm_ptr) {
      print(fd, "\n%M의 주위에 그런 것은 존재하지 않습니다.\n", crt_ptr);
      return(0);
   }

   if(ply_ptr->rom_num == crt_ptr->rom_num) {
     print(fd, "\n%M과 떨어져 있을 때만 사용 가능합니다.\n", crt_ptr);
     return(0);
   }
      /* 광장에서 혈마안 금지 */
  if(ply_ptr->rom_num==1001) {
        print(fd, "\n광장에서 혈마안을 하실수 없습니다.\n");
        return(0);
  }
   
   if(enm_ptr->type != PLAYER) {
      if(F_ISSET(enm_ptr, MUNKIL)) {
	 print(fd, "당신은 %s를 해칠 수 없습니다.\n",
	       F_ISSET(enm_ptr, MMALES) ? "그":"그녀");
	 return(0);
      }
      if(mrand(0,1) && (enm_ptr->piety < enm_ptr->piety) && F_ISSET(enm_ptr, MMGONL)) {
		  print(fd, "당신의 공격이 %M에게 아무소용이 없는듯 합니다.\n", crt_ptr);
	 return(0);
      }
      add_enm_crt(ply_ptr->name, enm_ptr);
   }

   print(fd, "\n천하에 무공 중 기이한 것도 많으니 여기 눈빛으로 살생을 하는 \n");
   print(fd, "혈마안이 있도다. 천하에 누가 있어 혈마안의 눈에 살아 남으리~~~\n");
   print(fd, "\"으으이야압~~~!!! 번쩍!!!\"\n");
   print(fd, "당신은 정신을 가다듬어멀리 떨어져 있는 %m에게 살광을 퍼뜨립니다.\n", enm_ptr);
   

   chance = (20- ply_ptr->thaco) - (20-crt_ptr->thaco) + (ply_ptr->level+29)/30 \
     + bonus[ply_ptr->intelligence]*3;
   chance = MIN(chance, 20);
   if(chance < 5) chance = 5;
/*   
   print(fd, "chance = %d \n", chance);
*/
   if (mrand(1,22) <= chance) {
     dmg = mdice(crt_ptr) * (5 + mrand(1,(ply_ptr->intelligence + ply_ptr->piety + chance)/10));
     dmg = MIN(enm_ptr->hpcur, dmg);
     enm_ptr->hpcur -= dmg*3;
     
     add_enm_dmg(ply_ptr->name, enm_ptr, dmg);

     if(enm_ptr->type != PLAYER && is_enm_crt(crt_ptr->name, enm_ptr)) {
       add_enm_dmg(crt_ptr->name, enm_ptr, dmg*2);
     }
     else {
       add_enm_crt(crt_ptr->name, enm_ptr);
       add_enm_dmg(crt_ptr->name, enm_ptr, dmg*2);
     }

		 print(fd, "당신은");
		 ANSI(fd, YELLOW);
		 print(fd, "혈마안 ");
		 ANSI(fd, WHITE);
		 ANSI(fd, NORMAL);
		 print(fd, "으로 %m에게 ", enm_ptr);
		 ANSI(fd, GREEN);
		 print(fd, "%d", dmg);
		 ANSI(fd, WHITE);
		 ANSI(fd, NORMAL);
		 print(fd, "점의 피해를 입혔습니다.\n");   


/*     print(fd,"\n당신은 혈마안으로 %m에게 %d의 피해를 입혔습니다.\n",  enm_ptr, dmg );    */
     
     broadcast_rom(fd, ply_ptr->rom_num,
		   "\n%M이 혈마안으로 %m에게 %d의 피해를 입혔습니다.\n", ply_ptr, enm_ptr, dmg);

     broadcast_rom(fd, crt_ptr->rom_num,
		   "\n%M이 혈마안으로 %M을 도와 %m에게 %d의 피해를 입혔습니다.\n", ply_ptr, crt_ptr, enm_ptr, dmg);

		 print(crt_ptr->fd, "당신은 %M의 " , ply_ptr);
		 ANSI(crt_ptr->fd, YELLOW);
		 print(crt_ptr->fd, "혈마안");
		 ANSI(crt_ptr->fd, WHITE);
		 ANSI(crt_ptr->fd, NORMAL);
		 print(crt_ptr->fd, "으로 %m에게 ", enm_ptr);
		 ANSI(crt_ptr->fd, GREEN);
		 print(crt_ptr->fd, "%d", dmg*2);
		 ANSI(crt_ptr->fd, WHITE);
		 ANSI(crt_ptr->fd, NORMAL);
		 print(crt_ptr->fd, "점의 피해를 입혔습니다.\n");

/*     print(crt_ptr->fd,"\n당신은 %M의 혈마안으로 %m에게 %d의 피해를 입혔습니다.\n", ply_ptr, enm_ptr, dmg*2 );    */
     
     if(enm_ptr->hpcur < 1) {
       print(fd, "\n당신은 혈마안으로 %m%j 죽였습니다.", enm_ptr,"3");
       print(crt_ptr->fd, "\n당신은 %M의 혈마안으로 %m%j 죽였습니다.", ply_ptr, enm_ptr,"3");
       broadcast_rom(fd, ply_ptr->rom_num,
		     "\n%M%j 혈마안으로 멀리 떨어져 있는 %m%j 죽였습니다.", ply_ptr,"1", enm_ptr,"3");
       die(enm_ptr, ply_ptr);
     }
     
   }
   else {
     print(fd,"\n당신의 혈마안이 실패했습니다.\n");
     broadcast_rom(fd, ply_ptr->rom_num,
		   "\n%M의 혈마안이 실패했습니다.\n", ply_ptr);
     
     ply_ptr->hpcur -= ply_ptr->hpcur/2;
     ply_ptr->mpcur -= ply_ptr->mpcur/2;
     
   }
   ply_red_eye_time[fd] = t+ (15 - MIN(10, ply_ptr->intelligence/3));  
   return(0);
}

/************************************************************************/
/*				천안술					*/
/************************************************************************/

int thief_stat(ply_ptr, cmnd)
creature	*ply_ptr;
cmd		*cmnd;
{
  room		*rom_ptr;
  object		*obj_ptr;
  creature	*crt_ptr;
  creature	*ply_ptr2;
  char		str[2048];
  int		fd, n, i, j, chance;
  long          t;

  fd = ply_ptr->fd;

  if(ply_ptr->class < INVINCIBLE) {
	 ANSI(fd, CYAN);
	 print(fd, "무적");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "이상만 쓸수 있는 기술입니다.\n");
	 return(0);
  }
  if(!S_ISSET(ply_ptr, STHIEF)) {

	 print(fd, "아직 ");
	 ANSI(fd, CYAN);
	 print(fd, "도둑");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "을 ");
	 ANSI(fd, CYAN);
	 print(fd, "무적수련");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "하지 않았습니다.\n");
	 return(0);
  }
  
  if(cmnd->num < 2) {
     print(fd, "\n무엇을 분석하시려구요?\n");
    return(0);
  }
  
  i = LT(ply_ptr, LT_STEAL);
  t = time(0);
  
  if(t < i) {
    please_wait(fd, i-t);
    return(0);
  }

  if(F_ISSET(ply_ptr, PBLIND)) {
    print(fd, "\n당신은 눈이 멀어 천안술을 펼칠 수 없습니다.");
    return(0);
  }
  if(ply_is_attacking(ply_ptr, cmnd)) {
    print(fd, "싸우는 도중에는 천안술을 펼칠 수 없습니다.");
    return(0);
  }
  
  if(F_ISSET(ply_ptr, PINVIS)) {
    F_CLR(ply_ptr, PINVIS);
    print(fd, "당신의 모습이 서서히 드러납니다.");
    broadcast_rom(fd, ply_ptr->rom_num, "\n%M의 모습이 서서히 드러납니다.",
		  ply_ptr);
  }

  print(fd, "\n무릇 도둑의 기술 중 백미는 훔치는 것보다 훔쳐 보는 것이니..\n");
  print(fd, "여기 훔쳐보기의 최고 기술 천안술이 있도다.\n");
  print(fd, "천하만물도 또한 천안술 앞에서는 숨김이 없도다.\n");
  print(fd, "당신은 안광을 집중해 천안술을 시전합니다.\n\n");

  ply_ptr->lasttime[LT_STEAL].ltime = t;
  ply_ptr->lasttime[LT_STEAL].interval = 20 - MIN(15, ply_ptr->dexterity/5 + ply_ptr->intelligence/3);

  if(cmnd->num < 3)
    ply_ptr2 = ply_ptr;
  else {
    ply_ptr2 = find_crt(ply_ptr, ply_ptr->parent_rom->first_mon,
			cmnd->str[2], cmnd->val[2]);
    cmnd->str[2][0] = up(cmnd->str[2][0]);
    if(!ply_ptr2)
      ply_ptr2 = find_crt(ply_ptr, ply_ptr->parent_rom->first_ply,
			  cmnd->str[2], cmnd->val[2]);
    if(!ply_ptr2)
      ply_ptr2 = find_who(cmnd->str[2]);
    if(!ply_ptr2 || ply_ptr2->rom_num != ply_ptr->rom_num || (ply_ptr->class<DM && 
					     F_ISSET(ply_ptr2, PDMINV)))
      ply_ptr2 = ply_ptr;
  }
  
  rom_ptr = ply_ptr2->parent_rom;
  
  /* Give info on object, if found */
  obj_ptr = find_obj(ply_ptr2, ply_ptr2->first_obj, cmnd->str[1], 
		     cmnd->val[1]);
  if(!obj_ptr) {
    for(i=0,j=0; i<MAXWEAR; i++) {
      if(EQUAL(ply_ptr2->ready[i], cmnd->str[1])) {
	j++;
	if(j == cmnd->val[1]) {
	  obj_ptr = ply_ptr2->ready[i];
	  break;
	}
      }
    }
  }

  if(!obj_ptr)
    obj_ptr = find_obj(ply_ptr2, rom_ptr->first_obj,
		       cmnd->str[1], cmnd->val[1]);
  
  if(obj_ptr) {

    chance = (25 + ((ply_ptr->level+3)/4)*10)-(((ply_ptr2->level+3)/4)*5);
    if (chance<30) chance=30;
    
    if(mrand(1,100) > chance) {
      print(fd, "%M의 소지품을 엿보는데 실패하였습니다!", ply_ptr2);

      if(ply_ptr2->type == PLAYER) {
	print( ply_ptr2->fd, "\n%M이 당신의 소지품을 슬쩍 엿봅니다.", ply_ptr);
	broadcast_rom2(fd, ply_ptr2->fd, ply_ptr->rom_num,
		       "\n%M이 %M의 소지품을 슬쩍 엿봅니다.", ply_ptr, ply_ptr2);
      }
      if(ply_ptr2->type == MONSTER && !F_ISSET(ply_ptr2, MUNKIL))  add_enm_crt(ply_ptr->name, ply_ptr2); 
    }
    else thief_stat_obj(ply_ptr, obj_ptr);

    return(0);
  }
  
  /*  Search for creature or player to get info on */
  crt_ptr = find_crt(ply_ptr, rom_ptr->first_mon, cmnd->str[1],
		     cmnd->val[1]);
  if(!crt_ptr) {
    crt_ptr = find_crt(ply_ptr, rom_ptr->first_ply, cmnd->str[1],
		       cmnd->val[1]);
    if(crt_ptr)
      print(fd, "\n다른 사용자의 신상정보를 알아낼 수 없습니다.\n");
    else
      print(fd, "그런건 없습니다.\n");

    return(0);
  }
  /*  cmnd->str[1][0] = up(cmnd->str[1][0]);
   */

  if(!F_ISSET(crt_ptr, MUNKIL)) {
    chance = (25 + ((ply_ptr->level+3)/4)*10)-(((crt_ptr->level+3)/4)*5);
    if (chance<30) chance=30;
    
    if(mrand(1,100) > chance && ply_ptr->class < crt_ptr->class) {
      print(fd, "%m의 신상정보를 알아보는데 실패했습니다!\n", crt_ptr);
      add_enm_crt(ply_ptr->name, crt_ptr); 
    }
    else 
      thief_stat_crt(ply_ptr, crt_ptr);
    return(0);
  }
  
  else
    print(fd, "%m의 신상정보를 알아낼 수 없습니다.\n", crt_ptr);
  
  return(0);
}

/************************************************************************/
/*				thief_stat_crt				*/
/************************************************************************/


int thief_stat_crt(ply_ptr, crt_ptr)
creature	*ply_ptr;
creature	*crt_ptr;
{
  FILE *fp;
  char alstr[16];
  char file[80];
  char str[15];
  int		i, fd, n, chance;
  
  fd = ply_ptr->fd;
  
  if(crt_ptr->alignment < -100)
    strcpy(alstr, " (악합니다)");
  else if(crt_ptr->alignment < 101)
    strcpy(alstr, " (평범합니다)");
  else
    strcpy(alstr, " (선합니다) ");
  
  if(!F_ISSET(crt_ptr , PMARRI)) strcpy(str ,"없음");
  else {
    sprintf(file, "%s/marriage/%s", PLAYERPATH, crt_ptr->name);
    fp = fopen(file, "r");
    fscanf(fp, "%s", str);
    fclose(fp);
  }
  
  if(crt_ptr->type == PLAYER && Ply[crt_ptr->fd].io) {
    print(fd, "\n[이  름] %s        [배우자] %s\n", crt_ptr->name, str);
    print(fd,   "[칭  호] %s\n\n", title_ply(crt_ptr,crt_ptr));
    
  }
  else
    print(fd, "[이  름] %s\n", crt_ptr->name);
  
  print(fd, "[레  벨] %-11d          [종  족] %s\n",
	crt_ptr->level, race_str[crt_ptr->race]);
  print(fd, "[직  업] %-11s          [성  향] %s %s\n\n",
	class_str[crt_ptr->class],
	F_ISSET(crt_ptr, PCHAOS) ? "악":"선", alstr);

  if(mrand(1, ply_ptr->intelligence) > 5) {  
    print(fd, "[체  력] %-5d/%-5d          [경험치] %lu\n",
	  crt_ptr->hpcur, crt_ptr->hpmax, crt_ptr->experience);
    print(fd, "[도  력] %-5d/%-5d          [  돈  ] %-7lu\n\n",
	  crt_ptr->mpcur, crt_ptr->mpmax, crt_ptr->gold);
  }

  if(mrand(1, ply_ptr->intelligence) > 10) {  
    print(fd, "[방어력] %-5d                [타  격] %d면 %d굴림 더하기 %d\n\n", (100-crt_ptr->armor), \
	  crt_ptr->ndice, crt_ptr->sdice,	crt_ptr->pdice);
  }

  if(mrand(1, ply_ptr->intelligence) > 15) {  
    print(fd, "[  힘  ] %-2d      [민  첩] %-2d      [맷  집] %-2d\n",
	  crt_ptr->strength, crt_ptr->dexterity, crt_ptr->constitution);
    print(fd, "[지  식] %-2d      [신앙심] %-2d      [용  기] %-2d\n\n",
	  crt_ptr->intelligence, crt_ptr->piety, (20-crt_ptr->thaco));
  }

  broadcast_rom2(fd, crt_ptr->fd, ply_ptr->rom_num,
		 "\n%M이 천안술로 %m의 신상정보를 알아냅니다.",
		 ply_ptr, crt_ptr);


  chance = (25 + ((ply_ptr->level+3)/4)*10)-(((crt_ptr->level+3)/4)*5);
  if (chance<0) chance=0;

  if(mrand(1,100) > chance) {
    print(fd, "\n%M의 소지품을 엿보는데 실패하였습니다!", crt_ptr);
    if(crt_ptr->type == MONSTER)  add_enm_crt(ply_ptr->name, crt_ptr); 
  }
  else {
    chance = MIN(90, 15 + ((ply_ptr->level+3)/4)*3);
    
    if(mrand(1,100) > chance && ply_ptr->class < SUB_DM && crt_ptr->type == PLAYER) {
      print(crt_ptr->fd, "%s님이 당신의 소지품을 슬쩍 엿봅니다.", ply_ptr);
      broadcast_rom2(fd, crt_ptr->fd, ply_ptr->rom_num,
		     "\n%M이 %m의 소지품을 슬쩍 엿봅니다.",
		     ply_ptr, crt_ptr);
    }
    
    sprintf(str, "[소지품] ");
    n = strlen(str);
    if(list_obj(&str[n], ply_ptr, crt_ptr->first_obj) > 0)
      print(fd, "%s", str);
    else
      print(fd, "[소지품] 없음\n");
  }
  
}

/************************************************************************/
/*				thief_stat_obj				*/
/************************************************************************/

int thief_stat_obj(ply_ptr, obj_ptr)
creature	*ply_ptr;
object		*obj_ptr;
{
  int	fd;
  char  str[1024];   
  fd = ply_ptr->fd;
  
  print(fd, "이름: %s\n", obj_ptr->name);

  print(fd, "설명: %s\n", obj_ptr->description);

  print(fd, "사용: %s\n", obj_ptr->use_output);
  
  print(fd, "사용회수 %d/%d\n", obj_ptr->shotscur, obj_ptr->shotsmax);
  
  print(fd, "종류: ");
  if(obj_ptr->type <= MISSILE) {
    switch(obj_ptr->type) {
    case SHARP: print(fd, "도"); break;
    case THRUST: print(fd, "검"); break;
    case BLUNT: print(fd, "봉"); break;
    case POLE: print(fd, "창"); break;
    case MISSILE: print(fd, "궁"); break;
    }
    print(fd, " 무기.\n");

    if(mrand(1, ply_ptr->intelligence) > 10) {  
      print(fd, "타격치: %d면 %d굴림 더하기 %d", obj_ptr->sdice, obj_ptr->ndice,
	    obj_ptr->pdice);
      if(obj_ptr->adjustment)
	print(fd, " (+%d)\n", obj_ptr->adjustment);
      else
	print(fd, "\n");
    }
  }
  else {
    switch(obj_ptr->type) {
    case ARMOR: 
      print(fd, "방어구");
      if(mrand(1, ply_ptr->intelligence) > 10) 	print(fd, "\n방어력: %2.2d", obj_ptr->armor); 
      break;
    case POTION: print(fd, "약"); break;
    case SCROLL: print(fd, "주문서"); break;
    case WAND: print(fd, "주문걸린 물건"); break;
    case CONTAINER: print(fd, "담는 종류"); break;
    case KEY: print(fd, "열쇠"); break;
    case LIGHTSOURCE: print(fd, "광원"); break;
    case LOTTERY: print(fd, "복권"); break;  
    case MISC: print(fd, "모르겠음"); break;
    }
    print(fd,"\n");
  }

  print(fd, "가격: %5.5d", obj_ptr->value);
  print(fd, "   무게: %2.2d", obj_ptr->weight);
  if(obj_ptr->questnum)
    print(fd, "임무: %d\n", obj_ptr->questnum);
  else
    print(fd, "\n");
  
  strcpy(str, "특성: ");
  if(F_ISSET(obj_ptr, ONOMAG)) strcat(str, "도술사 불제자 거부, ");
  if(F_ISSET(obj_ptr, OGOODO)) strcat(str, "선한 사람용, ");
  if(F_ISSET(obj_ptr, OEVILO)) strcat(str, "악한 사람용, ");
  if(F_ISSET(obj_ptr, OENCHA)) strcat(str, "빙의 되있음, ");
  if(F_ISSET(obj_ptr, ONOMAL)) strcat(str, "남성 금지, ");
  if(F_ISSET(obj_ptr, ONOFEM)) strcat(str, "여성 금지, ");

  if(strlen(str) > 11) {
    str[strlen(str)-2] = '.';
    str[strlen(str)-1] = 0;
  }
  else
    strcat(str, "특성 없음.");
  print(fd, "%s\n", str);

}

/*++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++*/
/*                  포박술                                      */
/*++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++*/

long ply_poback_time[PMAX];

int poback(ply_ptr, cmnd)
creature    *ply_ptr;
cmd     *cmnd;
{
  creature    *enm_ptr;
  room        *rom_ptr;
  room        *new_rom;
  exit_       *ext_ptr;
  char        file[80];

  int     fd, t, chance, chance1, dmg, dur, dur2, p, addprof;
  
  fd = ply_ptr->fd;

  if(fd < 0) return(0);
  
  if(ply_ptr->class < INVINCIBLE) {
	 ANSI(fd, CYAN);
	 print(fd, "무적");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "이상만 쓸수 있는 기술입니다.\n");
	 return(0);
  }
  if(!S_ISSET(ply_ptr, SRANGER)) {

	 print(fd, "아직 ");
	 ANSI(fd, CYAN);
	 print(fd, "포졸");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "을 ");
	 ANSI(fd, CYAN);
	 print(fd, "무적수련");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "하지 않았습니다.\n");
	 return(0);
  }
  
  if(cmnd->num < 3) {
    print(fd, "\n사용법 : 어디 몹이름 포박\n");
    return(0);
  }
  if(F_ISSET(ply_ptr, PBLIND)) {
    ANSI(fd, BOLD);
    ANSI(fd, RED);
    print(fd, "당신은 눈이 멀어 있습니다!");
    ANSI(fd, WHITE);
    ANSI(fd, NORMAL);
    return(0);
  }

   if(!ply_ptr->ready[WIELD-1] || !(ply_ptr->ready[WIELD-1]->type == BLUNT \
				    || ply_ptr->ready[WIELD-1]->type == POLE)) {

	 ANSI(fd, YELLOW);
	 print(fd, "포박술");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "을 구사하시려면 ");
	 ANSI(fd, RED);
	 print(fd, "봉");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "이나");
	 ANSI(fd, RED);
	 print(fd, "창");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "종류의 무기가 필요합니다.");
	 return(0);

/*     print(fd, "포박술을 구사하시려면  봉이나 창종류의 무기가 필요합니다.");    */
     return(0);
   }

  t = time(0);
   
  if(ply_poback_time[fd] > t) {
    please_wait(fd, ply_poback_time[fd] -t);
    return(0);
  }

  rom_ptr = ply_ptr->parent_rom;

  ext_ptr=find_ext(ply_ptr, rom_ptr->first_ext, cmnd->str[1], cmnd->val[1]);

  if(ext_ptr) {
    if(F_ISSET(ext_ptr, XCLOSD)) {
      print(fd, "그 출구는 닫혀 있습니다.");
      return(0);
    }
    sprintf(file, "%s/r%02d/r%05d", ROOMPATH,ext_ptr->room/1000, ext_ptr->room);
    if(!file_exists(file)) {
      print(fd, "지도가 없습니다.");
      return(0);
    }
    load_rom(ext_ptr->room, &new_rom);
    if(!new_rom || rom_ptr == new_rom) {
      print(fd, "지도가 없습니다.");
      return(0);
    }
    if(F_ISSET(new_rom, RONMAR) || F_ISSET(new_rom, RONFML)) {
      print(fd, "그 방은 볼 수가 없습니다.");
      return(0);
    }
    
    /*  display_rom(ply_ptr, new_rom); 
    return(0); */
  }
  else {
    print(fd,"\n%s쪽으로는 지도가 없습니다.", cmnd->str[1]);
    return(0);
  }

  ply_ptr->lasttime[LT_ATTCK].ltime = t;

  broadcast_rom(fd, ply_ptr->rom_num, 
		  "\n%M이 포박술을 구사하기 위해 정신을 집중합니다.\n", ply_ptr);

  enm_ptr = find_crt(ply_ptr, new_rom->first_mon, cmnd->str[2], cmnd->val[2]);

  if(!enm_ptr) {
    print(fd, "\n%s에 그런 것은 존재하지 않습니다.\n", new_rom->name);
    return(0);
  }
  
  if(enm_ptr->type != PLAYER) {
    if(F_ISSET(enm_ptr, MUNKIL)) {
      print(fd, "당신은 %s를 해칠 수 없습니다.\n",
	    F_ISSET(enm_ptr, MMALES) ? "그":"그녀");
      return(0);
    }
    if(mrand(0,1) && (enm_ptr->dexterity < enm_ptr->dexterity) && F_ISSET(enm_ptr, MMGONL)) {
      print(fd, "당신의 공격이 %M에게 아무소용이 없는듯 합니다.\n", enm_ptr);
      return(0);
    }
    add_enm_crt(ply_ptr->name, enm_ptr);
  }
  
  print(fd, "\n용투야의 난세에 살생이 들 끓는다.\n");
  print(fd, "이에 전설의 장인 동방천인이 만든 포승줄 파옥쇄가 있으니\n");
  print(fd, "세상에 부수지 못하고 묶어놓지 못하는게 없으며 죽을 때까지 풀리지\n");
  print(fd, "않으니 어느 누가 두려워 하지 않으리요~~~\n", enm_ptr);
  print(fd, "\n당신은 파옥쇄를 %m의 몸에 재빠르게 휘두릅니다.\n", enm_ptr);    

   chance = (20- ply_ptr->thaco) - (20-enm_ptr->thaco) + bonus[ply_ptr->intelligence]*2 \
     + bonus[ply_ptr->dexterity]*3;
   chance = MIN(chance, 20);
   if(chance < 5) chance = 5;

   if (mrand(1,22) <= chance) {

     if(ply_ptr->ready[WIELD-1]->shotscur > 0)
       ply_ptr->ready[WIELD-1]->shotscur--;
     
     if(ply_ptr->ready[WIELD-1]) {
       if(ply_ptr->ready[WIELD-1]->shotscur < 1) {
	 print(fd, "\n%S%j 부서져 버렸습니다.\n",
	       ply_ptr->ready[WIELD-1]->name,"1");
	 add_obj_crt(ply_ptr->ready[WIELD-1], ply_ptr);
	 ply_ptr->ready[WIELD-1] = 0;
	 return(0);
       }
     }

     dmg = ply_ptr->dexterity*2 + mdice(ply_ptr->ready[WIELD-1]);
     dmg = MIN(enm_ptr->hpcur, dmg);
     enm_ptr->hpcur -= dmg;

     if(ply_ptr->ready[WIELD-1]) {
       p = MIN(ply_ptr->ready[WIELD-1]->type, MISSILE);
       addprof = (dmg * enm_ptr->experience) / enm_ptr->hpmax;
       addprof = MIN(addprof, enm_ptr->experience);
       ply_ptr->proficiency[p] += addprof;
     }


     dur2 = chance;
     dur = dmg;


     if(F_ISSET(enm_ptr, PRMAGI) || F_ISSET(enm_ptr, MRMAGI) || F_ISSET(enm_ptr, MNOCHA)) {
       dur /= 3;
       dur2 /= 2;
     }
     if(mrand(0,1) || (ply_ptr->level < enm_ptr->level) && !F_ISSET(enm_ptr, MNOCHA)) {
       add_charm_crt(enm_ptr, ply_ptr);
       print(fd, "\n당신의 포박술로 %m의 정신이 혼미해집니다.\n", enm_ptr);
       enm_ptr->lasttime[LT_CHRMD].ltime = time(0);
       enm_ptr->lasttime[LT_CHRMD].interval = dur;
       F_SET(enm_ptr, MCHARM);
     }

     if(dur2 > 15) {
       enm_ptr->lasttime[LT_BEFUD].ltime = time(0);
       enm_ptr->lasttime[LT_BEFUD].interval = dur;
       F_SET(enm_ptr, MBEFUD);

       print(fd, "당신의 포박술이 %m의 정신을 혼수상태에 빠뜨립니다.\n", enm_ptr);
     }

     enm_ptr->lasttime[LT_ATTCK].ltime = time(0);
     enm_ptr->lasttime[LT_ATTCK].interval = dur2;

     enm_ptr->lasttime[LT_SPELL].ltime = time(0);
     enm_ptr->lasttime[LT_SPELL].interval = dmg;
     
     add_enm_dmg(ply_ptr->name, enm_ptr, dmg);

		 print(fd, "당신은 ");
		 ANSI(fd, YELLOW);
		 print(fd, "포박술");
		 ANSI(fd, WHITE);
		 ANSI(fd, NORMAL);
		 print(fd, "으로 %m에게 ", enm_ptr);
		 ANSI(fd, GREEN);
		 print(fd, "%d", dmg);
		 ANSI(fd, WHITE);
		 ANSI(fd, NORMAL);
		 print(fd, "점의 피해를 입혔습니다.\n");


/*     print(fd,"\n당신은 포박술로 %m에게 %d의 피해를 입혔습니다.\n",  enm_ptr, dmg );  */
    
     broadcast_rom(fd, enm_ptr->rom_num,
		   "\n%M이 포박술로 %m에게 %d의 피해를 입혔습니다.\n", ply_ptr, enm_ptr, dmg);

     if(enm_ptr->hpcur < 1) {
       print(fd, "\n당신은 포박술 도중 %m%j 죽였습니다.", enm_ptr,"3");
       broadcast_rom(fd, enm_ptr->rom_num,
		     "\n%M%j 포박술로  %m%j 죽였습니다.", ply_ptr,"1", enm_ptr,"3");
       broadcast_rom(fd, ply_ptr->rom_num,
		     "\n%M%j 포박술로 %s쪽에 있는 %m%j 죽였습니다.", ply_ptr,"1",ext_ptr->name,  enm_ptr,"3");
       die(enm_ptr, ply_ptr);
     }
     
   }
   else {
     print(fd,"\n당신의 포박술이 실패했습니다.\n");
     broadcast_rom(fd, ply_ptr->rom_num,
		   "\n%M의 포박술이 실패했습니다.\n", ply_ptr);
     ply_ptr->lasttime[LT_BEFUD].ltime = time(0);
     ply_ptr->lasttime[LT_BEFUD].interval = (20-chance)*5;
     F_SET(enm_ptr, MBEFUD);
     ply_ptr->lasttime[LT_CHRMD].ltime = time(0);
     ply_ptr->lasttime[LT_CHRMD].interval = (20-chance)*3;
     F_SET(enm_ptr, MCHARM);

     print(fd, "당신은 포박술의 실패로 혼수상태에 빠집니다.\n");
     
   }
   ply_poback_time[fd] = t+ (15 - MIN(8, ply_ptr->dexterity/4));  
   return(0);
}

/**********************************************************************/
/*                              정령소환술                            */
/**********************************************************************/


int angel(ply_ptr, cmnd)
creature        *ply_ptr;
cmd             *cmnd;
{
  long    i, t;
  int     chance, fd;
  
  fd = ply_ptr->fd;
  
  if(ply_ptr->class < INVINCIBLE && !(ply_ptr->class == MAGE && ply_ptr->level >= 50)) {
	 ANSI(fd, CYAN);
	 print(fd, "도술사");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, " 레벨 ");
	 ANSI(fd, CYAN);
	 print(fd, "50");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "이상만 쓸수 있는 기술입니다.\n");
	 return(0);
  }
  if(ply_ptr->class >= INVINCIBLE && !S_ISSET(ply_ptr, SMAGE)) {
	 print(fd, "아직 ");
	 ANSI(fd, CYAN);
	 print(fd, "도술사");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "를 ");
	 ANSI(fd, CYAN);
	 print(fd, "무적수련");
	 ANSI(fd, WHITE);
	 ANSI(fd, NORMAL);
	 print(fd, "하지 않았습니다.\n");
	 return(0);
  }
  
  if(F_ISSET(ply_ptr, PANGEL)) {
    print(fd, "당신은 지금 정령소환술을 사용중입니다.\n");
    return(0);
  }
  
  i = ply_ptr->lasttime[LT_ANGEL].ltime;
  t = time(0);
  
  if(t-i < 500L) {
    print(fd, "%d분 %02d초 기다리세요.\n",
	  (500L-t+i)/60L, (500L-t+i)%60L);
    return(0);
  }
  
  chance = MIN(85, ((ply_ptr->level+3)/4)*3 + bonus[ply_ptr->intelligence])*5;

  print(fd, "\n술사들의 최고 경지는 무릇 정령을 소환함이라~");
  print(fd, "\n여기 술사의 힘을 매꾸어 주는 정령이 있으니 바로 리매이노라~");
  print(fd, "\n\"나의 부름을 받은 정령이여 파천의 힘을~~~!\"");
  
  if(mrand(1,100) <= chance) {
    print(fd, "\n\n당신의 부름을 받은 정령이 주위를 맴돕니다.");
    broadcast_rom(fd, ply_ptr->rom_num, "\n%M이 정령을 소환합니다.", ply_ptr);
    F_SET(ply_ptr, PANGEL);
    ply_ptr->lasttime[LT_ANGEL].ltime = t;
    ply_ptr->lasttime[LT_ANGEL].interval = 300L;
  }
  else {
    print(fd, "당신은 정령을 소환하는데 실패했습니다.\n");
    broadcast_rom(fd, ply_ptr->rom_num, "%M이 정령소환술을 시도합니다.",
		  ply_ptr);
    ply_ptr->lasttime[LT_ANGEL].ltime = t - 500L;
  }
  
  return(0);
}













